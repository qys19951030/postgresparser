package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_Smoke verifies the skeleton entry point returns an empty report
// for statements that currently have no rules attached.
func TestCheck_Smoke(t *testing.T) {
	report, err := Check("INSERT INTO users (id) VALUES (1)")
	require.NoError(t, err)
	require.NotNil(t, report)
	assert.Empty(t, report.Findings)
	assert.Zero(t, report.HighCount)
	assert.Zero(t, report.MediumCount)
	assert.Zero(t, report.InfoCount)
}

// TestCheck_ParseError verifies parse failures surface the parser error
// instead of a fabricated empty report.
func TestCheck_ParseError(t *testing.T) {
	report, err := Check("SELECT FROM")
	require.Error(t, err)
	assert.Nil(t, report)
}

// TestCheckAll_Smoke verifies the batch entry point and aggregate counts.
func TestCheckAll_Smoke(t *testing.T) {
	batch, err := CheckAll("INSERT INTO t (id) VALUES (1); UPDATE t SET id = 2 WHERE id = 1;")
	require.NoError(t, err)
	require.NotNil(t, batch)
	require.Len(t, batch.Reports, 2)
	assert.Zero(t, batch.HighCount)
	assert.Zero(t, batch.MediumCount)
	assert.Zero(t, batch.InfoCount)
}
