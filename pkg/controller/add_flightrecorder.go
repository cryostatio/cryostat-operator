package controller

import (
	"github.com/rh-jmc-team/container-jfr-operator/pkg/controller/flightrecorder"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	return // FIXME
	AddToManagerFuncs = append(AddToManagerFuncs, flightrecorder.Add)
}
