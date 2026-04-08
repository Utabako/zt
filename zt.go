// Package zt provides Go bindings for the ZeroTier One core C API.
//
// This package wraps the ZeroTier node engine via CGo, providing a complete
// Go-idiomatic interface to create and manage ZeroTier network nodes.
package zt

/*
#cgo CFLAGS: -I${SRCDIR}/ZeroTierOne/include
#cgo LDFLAGS: -L${SRCDIR}/lib -lzerotiercore -lstdc++ -lm
#cgo !android LDFLAGS: -lpthread
#cgo android LDFLAGS: -llog
#include <stdbool.h>
#include "ZeroTierOne.h"
*/
import "C"
