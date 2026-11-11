package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetchAndGeocode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/geo" {
			if r.URL.Query().Get("name") == "Nowhere" {
				w.Write([]byte(`{}`))
				return
			}
			w.Write([]byte(`{"results":[{"name":"Львів","country_code":"UA","latitude":49.84,"longitude":24.03}]}`))
			return
		}
		if r.URL.Query().Get("temperature_unit") != "celsius" {
			w.WriteHeader(400)
			return
		}
		w.Write([]byte(`{"current":{"temperature_2m":3.5,"relative_humidity_2m":80,"apparent_temperature":1,"is_day":1,"weather_code":61,"wind_speed_10m":12,"wind_direction_10m":200,"uv_index":1},
			"daily":{"time":["2026-11-12","2026-11-13"],"weather_code":[61,0],"temperature_2m_max":[5,7],"temperature_2m_min":[1,2],"precipitation_sum":[4.2,0],"precipitation_probability_max":[90]}}`))
	}))
	defer srv.Close()
	forecastURL, geocodeURL = srv.URL+"/f", srv.URL+"/geo"
	ctx := context.Background()

	now, days, err := Fetch(ctx, 1, 2, "celsius")
	if err != nil || now.Temp != 3.5 || !now.IsDay || len(days) != 2 || days[0].RainProb != 90 || days[1].RainProb != 0 {
		t.Fatal(now, days, err)
	}
	p, err := Geocode(ctx, "Lviv", "uk")
	if err != nil || p.Name != "Львів" || p.Country != "UA" {
		t.Fatal(p, err)
	}
	if _, err := Geocode(ctx, "Nowhere", "uk"); err == nil {
		t.Fatal("want not found")
	}
}

func TestDescribeCompass(t *testing.T) {
	if _, k := Describe(61, true); k != "wmo.rain" {
		t.Fatal(k)
	}
	if i, _ := Describe(0, false); i != "🌙" {
		t.Fatal(i)
	}
	for deg, want := range map[int]string{0: "N", 44: "NE", 200: "S", 350: "N", 270: "W", -90: "W"} {
		if got := Compass(deg); got != want {
			t.Errorf("%d: %s want %s", deg, got, want)
		}
	}
}

// TECHWATCH_LIVE=1 go test ./weather -run Live -v
func TestLive(t *testing.T) {
	if os.Getenv("TECHWATCH_LIVE") == "" {
		t.Skip("set TECHWATCH_LIVE=1")
	}
	p, err := Geocode(context.Background(), "Львів", "uk")
	if err != nil {
		t.Fatal(err)
	}
	now, days, err := Fetch(context.Background(), p.Lat, p.Lon, "celsius")
	t.Log(p, now, len(days), err)
	if err != nil || len(days) != 7 {
		t.Fatal(err, len(days))
	}
}
