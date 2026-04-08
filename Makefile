# ZeroTier core C++ source directory
ZEROTIER_SRC ?= ./ZeroTierOne

# Output
LIB_DIR = lib
LIB_NAME = libzerotiercore.a

# Compiler settings
CXX ?= g++
AR ?= ar
CXXFLAGS = -O2 -fPIC -std=c++17 -Wall \
	-I$(ZEROTIER_SRC)/include \
	-I$(ZEROTIER_SRC) \
	-isystem $(ZEROTIER_SRC)/ext \
	-I$(ZEROTIER_SRC)/ext/prometheus-cpp-lite-1.0/core/include \
	-I$(ZEROTIER_SRC)/ext/prometheus-cpp-lite-1.0/3rdparty/http-client-lite/include \
	-I$(ZEROTIER_SRC)/ext/prometheus-cpp-lite-1.0/simpleapi/include \
	-include stdexcept \
	-DZT_USE_MINIUPNPC=0

# Source files
NODE_SRCS = $(wildcard $(ZEROTIER_SRC)/node/*.cpp)
OSDEP_SRCS = $(ZEROTIER_SRC)/osdep/OSUtils.cpp

ALL_SRCS = $(NODE_SRCS) $(OSDEP_SRCS)
OBJS = $(patsubst $(ZEROTIER_SRC)/%.cpp, build/%.o, $(ALL_SRCS))

.PHONY: all clean lib check-zerotier-src

all: lib

check-zerotier-src:
	@if [ ! -f "$(ZEROTIER_SRC)/include/ZeroTierOne.h" ]; then echo "Missing ZeroTierOne source at $(ZEROTIER_SRC). Run: git submodule update --init --recursive"; exit 1; fi
	@if [ ! -f "$(ZEROTIER_SRC)/osdep/OSUtils.cpp" ]; then echo "Invalid ZEROTIER_SRC=$(ZEROTIER_SRC): expected ZeroTierOne source tree"; exit 1; fi

lib: check-zerotier-src $(LIB_DIR)/$(LIB_NAME)

$(LIB_DIR)/$(LIB_NAME): $(OBJS)
	@mkdir -p $(LIB_DIR)
	$(AR) rcs $@ $^
	@echo "Built $@"

build/%.o: $(ZEROTIER_SRC)/%.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

clean:
	rm -rf build $(LIB_DIR)

# Cross-compilation targets
.PHONY: linux-amd64 linux-arm64 android-arm64

linux-amd64:
	$(MAKE) lib CXX=g++ CXXFLAGS="$(CXXFLAGS) -march=x86-64"

linux-arm64:
	$(MAKE) lib CXX=aarch64-linux-gnu-g++ AR=aarch64-linux-gnu-ar

android-arm64:
	@if [ -z "$$NDK" ]; then echo "Set NDK to Android NDK path"; exit 1; fi
	$(MAKE) lib \
		CXX="$$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android21-clang++" \
		AR="$$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin/llvm-ar" \
		CXXFLAGS="$(CXXFLAGS) -DANDROID"
