package controller

import (
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/recording"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, recording.Add)
}
