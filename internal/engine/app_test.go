package engine

import (
	"testing"
	"time"
)

func TestApplySyncSettingsClampsInterval(t *testing.T) {
	for _, test := range []struct {
		name     string
		interval time.Duration
		want     time.Duration
	}{
		{"below minimum", -time.Second, 30 * time.Second},
		{"minimum", 30 * time.Second, 30 * time.Second},
		{"within range", 5 * time.Minute, 5 * time.Minute},
		{"maximum", time.Hour, time.Hour},
		{"above maximum", 2 * time.Hour, time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			app := newTestApp()
			app.ApplySyncSettings(false, test.interval)
			settings := <-app.syncSettings
			if settings.AutoSync || settings.Interval != test.want {
				t.Fatalf("settings = %+v, want autoSync false, interval %s", settings, test.want)
			}
		})
	}
}
