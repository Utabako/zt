// Package zt provides Go bindings for the ZeroTier One core C API.
//
// This package wraps the ZeroTier node engine via CGo, providing a complete
// Go-idiomatic interface to create and manage ZeroTier network nodes.
package zt

/*
#cgo CFLAGS: -I${SRCDIR}/zerotier/include
#cgo CXXFLAGS: -I${SRCDIR}/zerotier/include
#cgo CXXFLAGS: -I${SRCDIR}/zerotier
#cgo CXXFLAGS: -I${SRCDIR}/zerotier/ext
#cgo CXXFLAGS: -I${SRCDIR}/zerotier/ext/prometheus-cpp-lite-1.0/core/include
#cgo CXXFLAGS: -I${SRCDIR}/zerotier/ext/prometheus-cpp-lite-1.0/3rdparty/http-client-lite/include
#cgo CXXFLAGS: -I${SRCDIR}/zerotier/ext/prometheus-cpp-lite-1.0/simpleapi/include
#cgo CXXFLAGS: -std=c++17
#cgo CXXFLAGS: -include stdexcept
#cgo CXXFLAGS: -DZT_USE_MINIUPNPC=0
#cgo LDFLAGS: -lstdc++ -lm
#cgo !android LDFLAGS: -lpthread
#cgo android LDFLAGS: -llog
#include <stdbool.h>
#include "ZeroTierOne.h"
*/
import "C"
