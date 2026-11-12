package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

var (
	forecastURL = "https://api.open-meteo.com/v1/forecast"
	geocodeURL  = "https://geocoding-api.open-meteo.com/v1/search"
	client      = &http.Client{Timeout: 15 * time.Second}
)

type Now struct {
	Temp, Feels, Wind, UV float64
	Humidity, WindDir     int
	Code                  int
	IsDay                 bool
}

type Day struct {
	Date     time.Time
	Max, Min float64
	Rain     float64
	RainProb int
	Code     int
}

func Fetch(ctx context.Context, lat, lon float64, unit string) (*Now, []Day, error) {
	q := url.Values{
		"latitude":         {fmt.Sprintf("%.4f", lat)},
		"longitude":        {fmt.Sprintf("%.4f", lon)},
		"current":          {"temperature_2m,relative_humidity_2m,apparent_temperature,is_day,weather_code,wind_speed_10m,wind_direction_10m,uv_index"},
		"daily":            {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,precipitation_probability_max"},
		"timezone":         {"auto"},
		"forecast_days":    {"7"},
		"temperature_unit": {unit},
	}
	var raw struct {
		Current struct {
			Temp     float64 `json:"temperature_2m"`
			Humidity int     `json:"relative_humidity_2m"`
			Feels    float64 `json:"apparent_temperature"`
			IsDay    int     `json:"is_day"`
			Code     int     `json:"weather_code"`
			Wind     float64 `json:"wind_speed_10m"`
			WindDir  int     `json:"wind_direction_10m"`
			UV       float64 `json:"uv_index"`
		} `json:"current"`
		Daily struct {
			Time     []string  `json:"time"`
			Code     []int     `json:"weather_code"`
			Max      []float64 `json:"temperature_2m_max"`
			Min      []float64 `json:"temperature_2m_min"`
			Rain     []float64 `json:"precipitation_sum"`
			RainProb []int     `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	if err := getJSON(ctx, forecastURL+"?"+q.Encode(), &raw); err != nil {
		return nil, nil, err
	}
	c := raw.Current
	now := &Now{Temp: c.Temp, Feels: c.Feels, Wind: c.Wind, UV: c.UV, Humidity: c.Humidity, WindDir: c.WindDir, Code: c.Code, IsDay: c.IsDay == 1}
	d := raw.Daily
	var days []Day
	for i, s := range d.Time {
		t, err := time.Parse("2006-01-02", s)
		if err != nil || i >= len(d.Code) || i >= len(d.Max) || i >= len(d.Min) {
			break
		}
		day := Day{Date: t, Max: d.Max[i], Min: d.Min[i], Code: d.Code[i]}
		if i < len(d.Rain) {
			day.Rain = d.Rain[i]
		}
		if i < len(d.RainProb) {
			day.RainProb = d.RainProb[i]
		}
		days = append(days, day)
	}
	return now, days, nil
}

var ErrNotFound = errors.New("city not found")

type Place struct {
	Name, Country string
	Lat, Lon      float64
}

func Geocode(ctx context.Context, city, lang string) (*Place, error) {
	q := url.Values{"name": {city}, "count": {"1"}, "language": {lang}, "format": {"json"}}
	var raw struct {
		Results []struct {
			Name    string  `json:"name"`
			Country string  `json:"country_code"`
			Lat     float64 `json:"latitude"`
			Lon     float64 `json:"longitude"`
		} `json:"results"`
	}
	if err := getJSON(ctx, geocodeURL+"?"+q.Encode(), &raw); err != nil {
		return nil, err
	}
	if len(raw.Results) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, city)
	}
	r := raw.Results[0]
	return &Place{r.Name, r.Country, r.Lat, r.Lon}, nil
}

func getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("open-meteo: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Icon and i18n key for a WMO weather code.
func Describe(code int, isDay bool) (icon, key string) {
	switch {
	case code == 0 && isDay:
		return "☀️", "wmo.clear"
	case code == 0:
		return "🌙", "wmo.clear"
	case code == 1:
		return "🌤️", "wmo.mainly_clear"
	case code == 2:
		return "⛅", "wmo.partly_cloudy"
	case code == 3:
		return "☁️", "wmo.overcast"
	case code == 45 || code == 48:
		return "🌫️", "wmo.fog"
	case code >= 51 && code <= 57:
		return "🌦️", "wmo.drizzle"
	case code >= 61 && code <= 67:
		return "🌧️", "wmo.rain"
	case code >= 71 && code <= 77:
		return "❄️", "wmo.snow"
	case code >= 80 && code <= 82:
		return "🌦️", "wmo.showers"
	case code == 85 || code == 86:
		return "🌨️", "wmo.snow"
	case code >= 95:
		return "⛈️", "wmo.storm"
	}
	return "🌡️", "wmo.unknown"
}

var compass = []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}

func Compass(deg int) string { return compass[((deg%360+360)%360+22)/45%8] }
