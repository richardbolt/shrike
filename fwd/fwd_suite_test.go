package fwd_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"

	"testing"
)

func init() {
	// Shut down logging to prevent test output pollution.
	log.SetLevel(log.FatalLevel)
}

func TestFwd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fwd Suite")
}
