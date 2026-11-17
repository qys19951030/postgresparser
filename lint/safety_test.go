package lint

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/valkdb/postgresparser"
)

// TestCheck_ConcurrentIndependent verifies concurrent calls do not share
// state or interfere with each other.
func TestCheck_ConcurrentIndependent(t *testing.T) {
	const goroutines = 32
	const iterations = 20

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				report, err := Check("SELECT * FROM users WHERE id > 1 ORDER BY id")
				if err != nil {
					errs <- err
					return
				}
				if len(report.Findings) == 0 {
					errs <- assert.AnError
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// TestCheck_DoesNotMutateIR verifies linting leaves the parsed IR untouched.
func TestCheck_DoesNotMutateIR(t *testing.T) {
	sql := "SELECT * FROM users CROSS JOIN orders ORDER BY id"
	before, err := postgresparser.ParseSQL(sql)
	require.NoError(t, err)

	snapshot := *before
	snapshotColumns := make([]postgresparser.SelectColumn, len(before.Columns))
	copy(snapshotColumns, before.Columns)
	snapshotTables := make([]postgresparser.TableRef, len(before.Tables))
	copy(snapshotTables, before.Tables)

	report, err := Check(sql)
	require.NoError(t, err)
	require.NotEmpty(t, report.Findings)

	assert.Equal(t, snapshotColumns, before.Columns)
	assert.Equal(t, snapshotTables, before.Tables)
	assert.Equal(t, snapshot.Where, before.Where)
	assert.Equal(t, snapshot.OrderBy, before.OrderBy)
	assert.Equal(t, snapshot.Limit, before.Limit)
}

// TestCheck_ReportCountsConsistent verifies the count fields always match the
// listed findings.
func TestCheck_ReportCountsConsistent(t *testing.T) {
	report, err := Check("SELECT * FROM users CROSS JOIN orders ORDER BY id")
	require.NoError(t, err)
	assert.Len(t, report.Findings, report.HighCount+report.MediumCount+report.InfoCount)
}
