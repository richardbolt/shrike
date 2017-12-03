package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestEdgy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HAL Suite")
}
