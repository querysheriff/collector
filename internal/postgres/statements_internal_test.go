package postgres

import "testing"

func TestCountersDecreased(t *testing.T) {
	t.Parallel()

	key := statementKey{databaseName: "app", userName: "u", queryID: 1}
	base := statementCounters{calls: 10, rows: 20, totalExecTime: 30, totalIOTime: 40}

	lower := func(mutate func(*statementCounters)) map[statementKey]statementCounters {
		c := base
		mutate(&c)

		return map[statementKey]statementCounters{key: c}
	}

	cases := []struct {
		name string
		cur  map[statementKey]statementCounters
		want bool
	}{
		{"identical", map[statementKey]statementCounters{key: base}, false},
		{"calls decreased", lower(func(c *statementCounters) { c.calls-- }), true},
		{"rows decreased", lower(func(c *statementCounters) { c.rows-- }), true},
		{"exec time decreased", lower(func(c *statementCounters) { c.totalExecTime-- }), true},
		{"io time decreased", lower(func(c *statementCounters) { c.totalIOTime-- }), true},
		{
			"new key only in cur is not a reset",
			map[statementKey]statementCounters{
				statementKey{databaseName: "app", userName: "u", queryID: 2}: base,
			},
			false,
		},
		{"key only in prev (aged out) is not a reset", map[statementKey]statementCounters{}, false},
	}

	prev := map[statementKey]statementCounters{key: base}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := countersDecreased(prev, c.cur); got != c.want {
				t.Errorf("countersDecreased = %v, want %v", got, c.want)
			}
		})
	}
}
