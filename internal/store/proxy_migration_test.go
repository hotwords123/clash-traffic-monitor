package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestDetailBackfillPreservesTotalsAndRunsOnce(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "already upgraded", true: "legacy schema"}[legacy], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "traffic.db")
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.db.Exec(`DELETE FROM schema_migrations;
 INSERT INTO traffic_minute VALUES
 (60,'single','site','',10,20,2),
 (60,'multi','site','',30,60,3),
 (60,'partial','site','',100,200,4),
 (60,'missing','site','',7,9,1),
 (60,'overrun','site','',5,5,1),
 (60,'mixed','site','',10,20,1),
 (60,'mixed','site','Group',3,4,1),
 (60,'empty','site','',10,20,2),
 (60,'zero','site','',0,0,1),
 (120,'single','site','',1,2,1);
 INSERT INTO connection_events VALUES
 ('a',60,'single','site',4,8,'','Group','{}'),
 ('b',60,'single','site',6,12,'','Group','{}'),
 ('c',60,'multi','site',10,20,'','Group','{}'),
 ('d',60,'multi','site',20,40,'','DIRECT','{}'),
 ('e',60,'partial','site',30,40,'','Group','{}'),
 ('f',60,'overrun','site',6,4,'','Group','{}'),
 ('g',60,'mixed','site',13,24,'','Group','{}'),
 ('h',60,'empty','site',4,8,'','Group','{}'),
 ('i',60,'empty','site',6,12,'','','{}'),
 ('j',60,'zero','site',0,0,'','DIRECT','{}');`)
			if err != nil {
				t.Fatal(err)
			}
			if legacy {
				// The old schema had only one aggregate for each minute/app/domain.
				_, err = db.db.Exec(`CREATE TABLE old_minutes(bucket INTEGER NOT NULL, app TEXT NOT NULL, domain TEXT NOT NULL, upload INTEGER NOT NULL, download INTEGER NOT NULL, connections INTEGER NOT NULL, PRIMARY KEY(bucket,app,domain));
 INSERT INTO old_minutes SELECT bucket,app,domain,SUM(upload),SUM(download),SUM(connections) FROM traffic_minute GROUP BY bucket,app,domain;
 DROP TABLE traffic_minute; ALTER TABLE old_minutes RENAME TO traffic_minute;`)
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := db.Summary(context.Background(), time.Unix(0, 0))
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			for attempt := 0; attempt < 2; attempt++ {
				db, err = Open(path)
				if err != nil {
					t.Fatal(err)
				}
				after, err := db.Summary(context.Background(), time.Unix(0, 0))
				if err != nil || after != before {
					t.Fatalf("totals changed: %+v -> %+v (%v)", before, after, err)
				}
				check := func(app, proxy string, want Summary) {
					t.Helper()
					got, err := db.Summary(context.Background(), time.Unix(60, 0), Filter{Until: time.Unix(120, 0), Apps: []string{app}, Proxies: []string{proxy}})
					if err != nil || got != want {
						t.Fatalf("%s/%s: %+v want %+v (%v)", app, proxy, got, want, err)
					}
				}
				check("single", "Group", Summary{Upload: 10, Download: 20, Connections: 2})
				check("single", "", Summary{})
				check("multi", "Group", Summary{Upload: 10, Download: 20})
				check("multi", "DIRECT", Summary{Upload: 20, Download: 40})
				check("multi", "", Summary{Connections: 3})
				check("partial", "Group", Summary{Upload: 30, Download: 40})
				check("partial", "", Summary{Upload: 70, Download: 160, Connections: 4})
				check("missing", "", Summary{Upload: 7, Download: 9, Connections: 1})
				check("overrun", "", Summary{Upload: 5, Download: 5, Connections: 1})
				check("empty", "Group", Summary{Upload: 4, Download: 8})
				check("empty", "", Summary{Upload: 6, Download: 12, Connections: 2})
				check("zero", "DIRECT", Summary{Connections: 1})
				if legacy {
					check("mixed", "Group", Summary{Upload: 13, Download: 24, Connections: 2})
				} else {
					check("mixed", "Group", Summary{Upload: 3, Download: 4, Connections: 1})
					check("mixed", "", Summary{Upload: 10, Download: 20, Connections: 1})
				}
				db.Close()
			}
		})
	}
}

func TestDetailBackfillRollsBackOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.db.Exec(`DELETE FROM schema_migrations;
 INSERT INTO traffic_minute VALUES(60,'app','site','',10,20,1);
 INSERT INTO connection_events VALUES('a',60,'app','site',10,20,'','Group','{}');
 CREATE TRIGGER fail_backfill BEFORE INSERT ON traffic_minute WHEN NEW.proxy != '' BEGIN SELECT RAISE(ABORT,'test failure'); END;`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if db, err = Open(path); err == nil {
		db.Close()
		t.Fatal("expected migration error")
	}
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var count, upload, download, connections int
	if err := raw.QueryRow(`SELECT COUNT(*),SUM(upload),SUM(download),SUM(connections) FROM traffic_minute`).Scan(&count, &upload, &download, &connections); err != nil {
		t.Fatal(err)
	}
	if count != 1 || upload != 10 || download != 20 || connections != 1 {
		t.Fatal("failed migration changed totals")
	}
	if err := raw.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed migration marked applied: %d %v", count, err)
	}
	if _, err := raw.Exec(`DROP TRIGGER fail_backfill`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := db.Summary(context.Background(), time.Unix(0, 0), Filter{Proxies: []string{"Group"}})
	if err != nil || got != (Summary{Upload: 10, Download: 20, Connections: 1}) {
		t.Fatalf("retry: %+v %v", got, err)
	}
}
