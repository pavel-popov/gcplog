package gcplog_test

import (
	"log"
	"os"
	"testing"

	"github.com/velppa/gcplog"
)

func TestStandardLoggerSatisfiesInterface(t *testing.T) {
	var l gcplog.Logger
	l = log.New(os.Stdout, "test ", log.LstdFlags)
	l.Printf("foo: %s", "bar")
}

func TestLogsAreSubmittedToGCP(t *testing.T) {
	l := gcplog.New(gcplog.Labels{"module": "test"})
	l.Printf("foo: %s", "bar")
	l.Debug("Test message from TestLogsAreSubmittedToGCP test",
		"number", 1,
		"string", "bar",
		"slice", []int{1, 2, 3},
	)
	l.Flush()
}
