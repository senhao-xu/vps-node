package repo

import (
	"testing"
	"time"
)

func TestServerCycleStart(t *testing.T) {
	tests := []struct {
		name     string
		now      time.Time
		resetDay int
		want     time.Time
	}{
		{
			name:     "after reset day",
			now:      time.Date(2026, time.March, 20, 12, 0, 0, 0, time.UTC),
			resetDay: 15,
			want:     time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "before reset day",
			now:      time.Date(2026, time.March, 3, 12, 0, 0, 0, time.UTC),
			resetDay: 15,
			want:     time.Date(2026, time.February, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "clamps short month",
			now:      time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC),
			resetDay: 31,
			want:     time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "previous short month from late date",
			now:      time.Date(2026, time.March, 30, 12, 0, 0, 0, time.UTC),
			resetDay: 31,
			want:     time.Date(2026, time.February, 28, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ServerCycleStart(test.now, test.resetDay); !got.Equal(test.want) {
				t.Fatalf("ServerCycleStart() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestServerMonthlyTotal(t *testing.T) {
	server := Server{TrafficAccounting: "max"}
	if got := serverMonthlyTotal(server, 40, 60); got != 60 {
		t.Fatalf("max accounting = %d, want 60", got)
	}
	server.TrafficAccounting = "sum"
	if got := serverMonthlyTotal(server, 40, 60); got != 100 {
		t.Fatalf("sum accounting = %d, want 100", got)
	}
}
