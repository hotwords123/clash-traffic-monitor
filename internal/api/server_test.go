package api

import (
	"net/http/httptest"
	"testing"
)

func TestQueryFilter(t *testing.T) {
	for _, test := range []struct {
		query string
		valid bool
	}{
		{"from=60&to=180&app=browser.exe&domain=example.com", true},
		{"from=61&to=180", false}, {"from=180&to=60", false},
		{"from=invalid", false}, {"to=-60", false}, {"from=0&to=999999999999", false},
	} {
		t.Run(test.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			_, _, ok := queryFilter(w, httptest.NewRequest("GET", "/api/series?"+test.query, nil))
			if ok != test.valid {
				t.Fatalf("valid=%v, response=%s", ok, w.Body.String())
			}
			if !ok && w.Code != 400 {
				t.Fatalf("status=%d", w.Code)
			}
		})
	}
}
