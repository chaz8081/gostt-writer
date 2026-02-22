package coreml

import (
	"math"
	"testing"
	"unsafe"
)

func TestNewTensor(t *testing.T) {
	tests := []struct {
		name  string
		shape []int64
		dtype DType
	}{
		{"1D float32", []int64{10}, DTypeFloat32},
		{"2D float32", []int64{3, 4}, DTypeFloat32},
		{"3D float32", []int64{2, 3, 4}, DTypeFloat32},
		{"2D int32", []int64{5, 5}, DTypeInt32},
		{"2D float16", []int64{2, 8}, DTypeFloat16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tensor, err := NewTensor(tt.shape, tt.dtype)
			if err != nil {
				t.Fatalf("NewTensor(%v, %v) returned error: %v", tt.shape, tt.dtype, err)
			}
			defer tensor.Close()

			if got := tensor.Rank(); got != len(tt.shape) {
				t.Errorf("Rank() = %d, want %d", got, len(tt.shape))
			}

			gotShape := tensor.Shape()
			if len(gotShape) != len(tt.shape) {
				t.Fatalf("Shape() length = %d, want %d", len(gotShape), len(tt.shape))
			}
			for i, dim := range gotShape {
				if dim != tt.shape[i] {
					t.Errorf("Shape()[%d] = %d, want %d", i, dim, tt.shape[i])
				}
			}

			if got := tensor.DType(); got != tt.dtype {
				t.Errorf("DType() = %d, want %d", got, tt.dtype)
			}
		})
	}
}

func TestNewTensorWithData(t *testing.T) {
	// Create float32 data
	data := []float32{1.0, 2.0, 3.0, 4.0, 5.0, 6.0}
	shape := []int64{2, 3}

	tensor, err := NewTensorWithData(shape, DTypeFloat32, unsafe.Pointer(&data[0]))
	if err != nil {
		t.Fatalf("NewTensorWithData returned error: %v", err)
	}
	defer tensor.Close()

	// Verify shape
	if got := tensor.Rank(); got != 2 {
		t.Errorf("Rank() = %d, want 2", got)
	}
	gotShape := tensor.Shape()
	if gotShape[0] != 2 || gotShape[1] != 3 {
		t.Errorf("Shape() = %v, want [2 3]", gotShape)
	}

	// Verify data round-trip
	ptr := tensor.DataPtr()
	if ptr == nil {
		t.Fatal("DataPtr() returned nil")
	}

	// Read back float32 values
	result := unsafe.Slice((*float32)(ptr), 6)
	for i, v := range result {
		if math.Abs(float64(v)-float64(data[i])) > 1e-6 {
			t.Errorf("data[%d] = %f, want %f", i, v, data[i])
		}
	}

	// Verify size in bytes
	expectedBytes := int64(6 * 4) // 6 float32 elements * 4 bytes each
	if got := tensor.SizeBytes(); got != expectedBytes {
		t.Errorf("SizeBytes() = %d, want %d", got, expectedBytes)
	}
}

func TestNewTensorWithDataInt32(t *testing.T) {
	data := []int32{10, 20, 30, 40}
	shape := []int64{4}

	tensor, err := NewTensorWithData(shape, DTypeInt32, unsafe.Pointer(&data[0]))
	if err != nil {
		t.Fatalf("NewTensorWithData returned error: %v", err)
	}
	defer tensor.Close()

	if got := tensor.DType(); got != DTypeInt32 {
		t.Errorf("DType() = %d, want DTypeInt32 (%d)", got, DTypeInt32)
	}

	ptr := tensor.DataPtr()
	result := unsafe.Slice((*int32)(ptr), 4)
	for i, v := range result {
		if v != data[i] {
			t.Errorf("data[%d] = %d, want %d", i, v, data[i])
		}
	}
}

func TestLoadModelBadPath(t *testing.T) {
	_, err := LoadModel("/nonexistent/path/to/model.mlmodelc")
	if err == nil {
		t.Fatal("LoadModel with nonexistent path should return error")
	}
	t.Logf("Got expected error: %v", err)
}

func TestCompileModelBadPath(t *testing.T) {
	_, err := CompileModel("/nonexistent/path/to/model.mlpackage", "")
	if err == nil {
		t.Fatal("CompileModel with nonexistent path should return error")
	}
	t.Logf("Got expected error: %v", err)
}

func TestComputeUnits(t *testing.T) {
	// Just verify these don't panic
	SetComputeUnits(ComputeAll)
	SetComputeUnits(ComputeCPUOnly)
	SetComputeUnits(ComputeCPUAndGPU)
	SetComputeUnits(ComputeCPUAndANE)
	// Reset to default
	SetComputeUnits(ComputeAll)
}

func TestTensorDim(t *testing.T) {
	shape := []int64{3, 5, 7}
	tensor, err := NewTensor(shape, DTypeFloat32)
	if err != nil {
		t.Fatalf("NewTensor returned error: %v", err)
	}
	defer tensor.Close()

	for i, expected := range shape {
		if got := tensor.Dim(i); got != expected {
			t.Errorf("Dim(%d) = %d, want %d", i, got, expected)
		}
	}

	// Out of bounds should return 0
	if got := tensor.Dim(3); got != 0 {
		t.Errorf("Dim(3) = %d, want 0 (out of bounds)", got)
	}
	if got := tensor.Dim(-1); got != 0 {
		t.Errorf("Dim(-1) = %d, want 0 (negative index)", got)
	}
}

func TestDTypeConstants(t *testing.T) {
	// Verify the dtype constants are distinct
	dtypes := []DType{DTypeFloat32, DTypeFloat16, DTypeInt32, DTypeInt64, DTypeBool}
	seen := make(map[DType]bool)
	for _, dt := range dtypes {
		if seen[dt] {
			t.Errorf("Duplicate DType value: %d", dt)
		}
		seen[dt] = true
	}
}

func TestComputeUnitsConstants(t *testing.T) {
	// Verify the compute unit constants are distinct
	units := []ComputeUnits{ComputeAll, ComputeCPUOnly, ComputeCPUAndGPU, ComputeCPUAndANE}
	seen := make(map[ComputeUnits]bool)
	for _, u := range units {
		if seen[u] {
			t.Errorf("Duplicate ComputeUnits value: %d", u)
		}
		seen[u] = true
	}
}
