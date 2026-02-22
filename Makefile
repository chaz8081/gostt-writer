# gostt-writer Makefile
# Build system for local dictation app using whisper.cpp

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build_go
MODELS_DIR := models
BIN_DIR := bin
BINARY := $(BIN_DIR)/gostt-writer

# whisper.cpp paths (match the Go bindings Makefile conventions)
INCLUDE_PATH := $(abspath $(WHISPER_DIR)/include):$(abspath $(WHISPER_DIR)/ggml/include)
GGML_METAL_PATH_RESOURCES := $(abspath $(WHISPER_DIR))

# Library paths: whisper, ggml, ggml-blas, ggml-metal
LIBRARY_PATH_DARWIN := $(abspath $(WHISPER_BUILD)/src):$(abspath $(WHISPER_BUILD)/ggml/src):$(abspath $(WHISPER_BUILD)/ggml/src/ggml-blas):$(abspath $(WHISPER_BUILD)/ggml/src/ggml-metal)

# macOS linker flags
EXT_LDFLAGS := -framework Foundation -framework Metal -framework MetalKit -lggml-metal -lggml-blas

# Model URL
MODEL_NAME := ggml-base.en.bin
MODEL_URL := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/$(MODEL_NAME)
MODEL_PATH := $(MODELS_DIR)/$(MODEL_NAME)

.PHONY: all whisper model build run clean test help

all: whisper model build

help:
	@echo "gostt-writer build targets:"
	@echo "  make whisper  - Build whisper.cpp static library (Metal + Accelerate)"
	@echo "  make model    - Download ggml-base.en.bin model"
	@echo "  make build    - Build gostt-writer binary"
	@echo "  make run      - Build and run"
	@echo "  make test     - Run all tests"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make all      - whisper + model + build"

whisper:
	@echo "Building whisper.cpp with Metal + Accelerate..."
	cmake -S $(WHISPER_DIR) -B $(WHISPER_DIR)/build_go \
		-DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF
	cmake --build $(WHISPER_DIR)/build_go --target whisper
	@echo "whisper.cpp build complete."

model: $(MODEL_PATH)

$(MODEL_PATH):
	@echo "Downloading $(MODEL_NAME)..."
	@mkdir -p $(MODELS_DIR)
	curl -L -o $(MODEL_PATH) $(MODEL_URL)
	@echo "Model downloaded to $(MODEL_PATH)"

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 \
	C_INCLUDE_PATH=$(INCLUDE_PATH) \
	LIBRARY_PATH=$(LIBRARY_PATH_DARWIN) \
	GGML_METAL_PATH_RESOURCES=$(GGML_METAL_PATH_RESOURCES) \
	go build -ldflags "-extldflags '-framework Foundation -framework Metal -framework MetalKit -lggml-metal -lggml-blas'" \
		-o $(BINARY) ./cmd/gostt-writer
	@echo "Built $(BINARY)"

run: build
	./$(BINARY)

test:
	CGO_ENABLED=1 \
	C_INCLUDE_PATH=$(INCLUDE_PATH) \
	LIBRARY_PATH=$(LIBRARY_PATH_DARWIN) \
	GGML_METAL_PATH_RESOURCES=$(GGML_METAL_PATH_RESOURCES) \
	go test -ldflags "-extldflags '-framework Foundation -framework Metal -framework MetalKit -lggml-metal -lggml-blas'" \
		-v ./...

clean:
	rm -rf $(BIN_DIR)
	rm -rf $(WHISPER_DIR)/build_go
	go clean
	@echo "Cleaned build artifacts."
