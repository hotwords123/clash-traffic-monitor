package store

// Backfill once, before collection starts. Never derive routes from domain names.
// A bucket already containing routed totals is skipped because its events can
// contain both old and new samples, which cannot be separated reliably.
func (s *Store) backfillProxyBuckets() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY)`); err != nil {
		return err
	}
	const migration = "proxy-detail-backfill-v1"
	var applied int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE name = ?`, migration).Scan(&applied); err != nil {
		return err
	}
	if applied != 0 {
		return nil
	}
	_, err = tx.Exec(`
 CREATE TEMP TABLE proxy_backfill AS
 WITH detail_totals AS (
   SELECT observed_at AS bucket, app, domain, SUM(upload) AS upload, SUM(download) AS download,
     COUNT(DISTINCT proxy) AS routes, MIN(proxy) AS route, MIN(upload) AS min_upload, MIN(download) AS min_download
   FROM connection_events GROUP BY observed_at, app, domain
 ), eligible AS (
   SELECT t.bucket, t.app, t.domain,
     CASE WHEN d.routes = 1 AND d.route != '' AND d.upload = t.upload AND d.download = t.download
       THEN t.connections ELSE 0 END AS connections
   FROM traffic_minute t JOIN detail_totals d USING(bucket, app, domain)
   WHERE t.proxy = '' AND d.min_upload >= 0 AND d.min_download >= 0
     AND d.upload <= t.upload AND d.download <= t.download
     AND NOT EXISTS (SELECT 1 FROM traffic_minute routed
       WHERE routed.bucket = t.bucket AND routed.app = t.app AND routed.domain = t.domain AND routed.proxy != '')
 )
 SELECT e.observed_at AS bucket, e.app, e.domain, e.proxy,
   SUM(e.upload) AS upload, SUM(e.download) AS download, eligible.connections
 FROM connection_events e JOIN eligible ON e.observed_at = eligible.bucket AND e.app = eligible.app AND e.domain = eligible.domain
 WHERE e.proxy != '' GROUP BY e.observed_at, e.app, e.domain, e.proxy;
 CREATE INDEX idx_proxy_backfill_bucket ON proxy_backfill(bucket, app, domain);

 INSERT INTO traffic_minute(bucket, app, domain, proxy, upload, download, connections)
 SELECT bucket, app, domain, proxy, upload, download, connections FROM proxy_backfill;

 UPDATE traffic_minute AS t SET
   upload = t.upload - moved.upload,
   download = t.download - moved.download,
   connections = t.connections - moved.connections
 FROM (SELECT bucket, app, domain, SUM(upload) AS upload, SUM(download) AS download, SUM(connections) AS connections
       FROM proxy_backfill GROUP BY bucket, app, domain) AS moved
 WHERE t.proxy = '' AND t.bucket = moved.bucket AND t.app = moved.app AND t.domain = moved.domain;

 DELETE FROM traffic_minute WHERE proxy = '' AND upload = 0 AND download = 0 AND connections = 0
   AND EXISTS (SELECT 1 FROM proxy_backfill b WHERE b.bucket = traffic_minute.bucket AND b.app = traffic_minute.app AND b.domain = traffic_minute.domain);
 DROP TABLE proxy_backfill;
 `)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO schema_migrations(name) VALUES(?)`, migration); err != nil {
		return err
	}
	return tx.Commit()
}
