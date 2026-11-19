//go:build cgo

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/services"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExchangeConfigReportsDefaultsButNotReadFailures(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	configs := repository.NewSystemConfigRepository(db)
	handler := NewExchangeHandler(services.NewExchangeService(nil, nil, nil, nil, configs, nil, nil, nil, nil), nil)
	router := gin.New()
	router.GET("/admin", handler.GetExchangeConfig)
	router.GET("/public", handler.GetExchangeConfigPublic)
	check := func(status int, enabled, monthly bool) {
		t.Helper()
		for _, path := range []string{"/admin", "/public"} {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != status {
				t.Fatalf("%s status=%d want=%d", path, recorder.Code, status)
			}
			if status != http.StatusOK {
				continue
			}
			var response struct {
				Data GetExchangeConfigResponse `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.Enabled != enabled || (path == "/admin" && response.Data.ExchangeMonthlyEnabled != monthly) {
				t.Fatalf("%s wrong config: %+v err=%v", path, response.Data, err)
			}
		}
	}
	check(http.StatusOK, true, false)
	if err := configs.Set("exchange_enabled", "false"); err != nil {
		t.Fatal(err)
	}
	if err := configs.Set("exchange_monthly_enabled", "yes"); err != nil {
		t.Fatal(err)
	}
	check(http.StatusOK, false, true)
	if err := db.Migrator().DropTable(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	check(http.StatusInternalServerError, false, false)
}
