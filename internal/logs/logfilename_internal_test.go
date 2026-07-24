package logs

import "testing"

func TestRotatedLogMatcher(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		logFilename string
		matches     []string
		rejects     []string
	}{
		{
			name:        "default timestamp template",
			logFilename: "postgresql-%Y-%m-%d_%H%M%S.log",
			matches: []string{
				"postgresql-2026-07-24_120000.json",
				"postgresql-2026-07-24_120000.log",
			},
			rejects: []string{
				"repmgr.log",
				"pgbouncer.log",
				"backup.json",
				"postgresql-2026-07-24_120000.csv",
				"mypostgresql-2026-07-24_120000.json",
				"postgresql-2026-07-24_120000.json.gz",
			},
		},
		{
			name:        "weekday template",
			logFilename: "postgresql-%a.log",
			matches:     []string{"postgresql-Mon.json", "postgresql-Tue.log"},
			rejects:     []string{"postgresql-2.json", "repmgr.log"},
		},
		{
			name:        "no literal prefix",
			logFilename: "%Y-%m-%d.log",
			matches:     []string{"2026-07-24.json", "2026-07-24.log"},
			rejects:     []string{"repmgr.log", "notes.json"},
		},
		{
			name:        "custom prefix, no .log suffix in template",
			logFilename: "pg_%Y%m%d",
			matches:     []string{"pg_20260724.json", "pg_20260724.log"},
			rejects:     []string{"other_20260724.json"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			re, err := rotatedLogMatcher(tc.logFilename)
			if err != nil {
				t.Fatalf("rotatedLogMatcher(%q): %v", tc.logFilename, err)
			}
			for _, name := range tc.matches {
				if !re.MatchString(name) {
					t.Errorf("%q should match template %q", name, tc.logFilename)
				}
			}
			for _, name := range tc.rejects {
				if re.MatchString(name) {
					t.Errorf("%q should NOT match template %q", name, tc.logFilename)
				}
			}
		})
	}
}
