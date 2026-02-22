# gostt-writer Makefile
# Build system for local dictation app using whisper.cpp

WHISPER_DIR := third_party/whisper.cpp
WHISPER_BUILD := $(WHISPER_DIR)/build_go
MODELS_DIR := models
BIN_DIR := bin
BINARY := $(BIN_DIR)/gostt-writer
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

# whisper.cpp paths (match the Go bindings Makefile conventions)
INCLUDE_PATH := $(abspath $(WHISPER_DIR)/include):$(abspath $(WHISPER_DIR)/ggml/include)
GGML_METAL_PATH_RESOURCES := $(abspath $(WHISPER_DIR))

# Library paths: whisper, ggml, ggml-blas, ggml-metal
LIBRARY_PATH_DARWIN := $(abspath $(WHISPER_BUILD)/src):$(abspath $(WHISPER_BUILD)/ggml/src):$(abspath $(WHISPER_BUILD)/ggml/src/ggml-blas):$(abspath $(WHISPER_BUILD)/ggml/src/ggml-metal)

# macOS linker flags
EXT_LDFLAGS := -framework Foundation -framework Metal -framework MetalKit -framework CoreML -lggml-metal -lggml-blas

# Model URL
MODEL_NAME := ggml-base.en.bin
MODEL_URL := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/$(MODEL_NAME)
MODEL_PATH := $(MODELS_DIR)/$(MODEL_NAME)

# Parakeet TDT CoreML model
PARAKEET_DIR := $(MODELS_DIR)/parakeet-tdt-v2
PARAKEET_REPO := https://huggingface.co/FluidInference/parakeet-tdt-0.6b-v2-coreml

.PHONY: all whisper model parakeet-model build run clean test help

all: whisper model build

help:
	@echo "gostt-writer build targets:"
	@echo "  make whisper  - Build whisper.cpp static library (Metal + Accelerate)"
	@echo "  make model    - Download ggml-base.en.bin model"
	@echo "  make parakeet-model - Download Parakeet TDT CoreML models"
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

parakeet-model:
	@echo "Downloading Parakeet TDT v2 CoreML models..."
	@if [ -d "$(PARAKEET_DIR)/Encoder.mlmodelc" ]; then \
		echo "Parakeet models already present at $(PARAKEET_DIR)"; \
		exit 0; \
	fi
	@mkdir -p $(PARAKEET_DIR)
	@TMPDIR=$$(mktemp -d) && \
	echo "Cloning from HuggingFace (sparse checkout)..." && \
	git clone --filter=blob:none --no-checkout $(PARAKEET_REPO) $$TMPDIR && \
	cd $$TMPDIR && \
	git sparse-checkout set Preprocessor.mlmodelc Encoder.mlmodelc Decoder.mlmodelc JointDecision.mlmodelc parakeet_vocab.json && \
	git checkout && \
	git lfs pull && \
	echo "Copying models to $(PARAKEET_DIR)..." && \
	cp -R Preprocessor.mlmodelc Encoder.mlmodelc Decoder.mlmodelc JointDecision.mlmodelc parakeet_vocab.json $(abspath $(PARAKEET_DIR))/ && \
	cd - > /dev/null && \
	rm -rf $$TMPDIR && \
	echo "Parakeet models downloaded to $(PARAKEET_DIR)"

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 \
	C_INCLUDE_PATH=$(INCLUDE_PATH) \
	LIBRARY_PATH=$(LIBRARY_PATH_DARWIN) \
	GGML_METAL_PATH_RESOURCES=$(GGML_METAL_PATH_RESOURCES) \
	go build -ldflags "-X main.version=$(VERSION) -extldflags '$(EXT_LDFLAGS)'" \
		-o $(BINARY) ./cmd/gostt-writer
	@echo "Built $(BINARY)"

run: build
	./$(BINARY)

test:
	CGO_ENABLED=1 \
	C_INCLUDE_PATH=$(INCLUDE_PATH) \
	LIBRARY_PATH=$(LIBRARY_PATH_DARWIN) \
	GGML_METAL_PATH_RESOURCES=$(GGML_METAL_PATH_RESOURCES) \
	go test -ldflags "-extldflags '$(EXT_LDFLAGS)'" \
		-v ./...

clean:
	rm -rf $(BIN_DIR)
	rm -rf $(WHISPER_DIR)/build_go
	go clean
	@echo "Cleaned build artifacts."
