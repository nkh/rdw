package selftest_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nkh/rdw/internal/selftest"
	"github.com/stretchr/testify/assert"
)

func TestRun_AllPass(t *testing.T) {
	var buf bytes.Buffer
	result := selftest.Run(context.Background(), &buf)

	output := buf.String()
	t.Log(output)

	assert.True(t, result.Passed, "selftest failed:\n"+output)

	for _, c := range result.Checks {
		assert.True(t, c.Passed, "check %q failed: %s", c.Name, c.Detail)
	}
}

func TestRun_OutputContainsAllCheckNames(t *testing.T) {
	var buf bytes.Buffer
	result := selftest.Run(context.Background(), &buf)

	output := buf.String()

	for _, c := range result.Checks {
		assert.True(t, strings.Contains(output, c.Name),
			"output missing check name %q", c.Name)
	}
}

func TestRun_PassLines_HavePASSPrefix(t *testing.T) {
	var buf bytes.Buffer
	selftest.Run(context.Background(), &buf)

	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		assert.True(t,
			strings.HasPrefix(line, "[PASS]") || strings.HasPrefix(line, "[FAIL]"),
			"unexpected line format: %q", line)
	}
}
