package recording_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRecording(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Recording Suite")
}
