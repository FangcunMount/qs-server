package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestAnalysisStoreFilterParsing(t *testing.T) {
	for _, query := range []string{"store_ids=0", "store_ids=-1", "store_ids=10,,20", "store_ids=oops", "store_ids=18446744073709551616"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?"+query, nil)
		if _, err := parseOperationsFilter(c); err == nil {
			t.Fatalf("accepted %s", query)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/?store_ids=636809255101411886&from=2026-09-01&to=2026-09-13", nil)
	f, err := parseOperationsFilter(c)
	if err != nil || len(f.StoreIDs) != 1 || f.StoreIDs[0] != 636809255101411886 || f.To != "2026-09-13" {
		t.Fatalf("invalid parsing: %+v %v", f, err)
	}
}
