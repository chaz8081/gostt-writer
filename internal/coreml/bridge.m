// Vendored from gomlx/go-coreml (Apache 2.0 License)
// https://github.com/gomlx/go-coreml
// Original: internal/bridge/bridge.m

// bridge.m - Objective-C implementation of CoreML bridge
// This file wraps CoreML APIs and exposes them via C functions.

#import <Foundation/Foundation.h>
#import <CoreML/CoreML.h>
#include "bridge.h"
#include <string.h>

// Global compute units setting
static MLComputeUnits g_computeUnits = MLComputeUnitsAll;

void coreml_set_compute_units(CoreMLComputeUnits units) {
    switch (units) {
        case COREML_COMPUTE_CPU_ONLY:
            g_computeUnits = MLComputeUnitsCPUOnly;
            break;
        case COREML_COMPUTE_CPU_AND_GPU:
            g_computeUnits = MLComputeUnitsCPUAndGPU;
            break;
        case COREML_COMPUTE_CPU_AND_ANE:
            g_computeUnits = MLComputeUnitsCPUAndNeuralEngine;
            break;
        case COREML_COMPUTE_ALL:
        default:
            g_computeUnits = MLComputeUnitsAll;
            break;
    }
}

static void set_error(CoreMLError* error, int code, NSError* nsError) {
    if (error == NULL) return;
    error->code = code;
    if (nsError != nil) {
        // Capture full error chain: localizedDescription + underlying errors
        NSMutableString* msg = [NSMutableString stringWithString:[nsError localizedDescription]];
        NSError* underlying = nsError.userInfo[NSUnderlyingErrorKey];
        while (underlying != nil) {
            [msg appendFormat:@"\n  caused by [%@ %ld]: %@",
                underlying.domain, (long)underlying.code, [underlying localizedDescription]];
            underlying = underlying.userInfo[NSUnderlyingErrorKey];
        }
        error->message = strdup([msg UTF8String]);
    } else {
        error->message = NULL;
    }
}

char* coreml_compile_model(const char* package_path, const char* output_dir, CoreMLError* error) {
    @autoreleasepool {
        NSString* nsPackagePath = [NSString stringWithUTF8String:package_path];
        NSURL* packageURL = [NSURL fileURLWithPath:nsPackagePath];

        NSError* nsError = nil;

        // Compile the model using MLModel.compileModel(at:)
        NSURL* compiledURL = [MLModel compileModelAtURL:packageURL error:&nsError];
        if (compiledURL == nil) {
            set_error(error, 1, nsError);
            return NULL;
        }

        // If output_dir is specified, move the compiled model there
        if (output_dir != NULL && strlen(output_dir) > 0) {
            NSString* nsOutputDir = [NSString stringWithUTF8String:output_dir];
            NSString* destPath = [nsOutputDir stringByAppendingPathComponent:[compiledURL lastPathComponent]];
            NSURL* destURL = [NSURL fileURLWithPath:destPath];

            NSFileManager* fm = [NSFileManager defaultManager];

            // Remove existing if any
            [fm removeItemAtURL:destURL error:nil];

            // Move compiled model to destination
            if (![fm moveItemAtURL:compiledURL toURL:destURL error:&nsError]) {
                set_error(error, 2, nsError);
                return NULL;
            }

            return strdup([destPath UTF8String]);
        }

        // Return the temporary compiled path
        return strdup([[compiledURL path] UTF8String]);
    }
}

CoreMLModel coreml_load_model(const char* path, CoreMLError* error) {
    @autoreleasepool {
        NSString* nsPath = [NSString stringWithUTF8String:path];
        NSURL* url = [NSURL fileURLWithPath:nsPath];

        NSError* nsError = nil;

        // Configure model
        MLModelConfiguration* config = [[MLModelConfiguration alloc] init];
        config.computeUnits = g_computeUnits;

        MLModel* model = [MLModel modelWithContentsOfURL:url configuration:config error:&nsError];
        if (model == nil) {
            set_error(error, 1, nsError);
            return NULL;
        }

        // Return retained model
        return (__bridge_retained void*)model;
    }
}

void coreml_free_model(CoreMLModel model) {
    if (model != NULL) {
        MLModel* m = (__bridge_transfer MLModel*)model;
        (void)m; // ARC will release
    }
}

int coreml_model_input_count(CoreMLModel model) {
    @autoreleasepool {
        MLModel* m = (__bridge MLModel*)model;
        return (int)[[m modelDescription].inputDescriptionsByName count];
    }
}

int coreml_model_output_count(CoreMLModel model) {
    @autoreleasepool {
        MLModel* m = (__bridge MLModel*)model;
        return (int)[[m modelDescription].outputDescriptionsByName count];
    }
}

const char* coreml_model_input_name(CoreMLModel model, int index) {
    @autoreleasepool {
        MLModel* m = (__bridge MLModel*)model;
        NSArray* keys = [[[m modelDescription].inputDescriptionsByName allKeys] sortedArrayUsingSelector:@selector(compare:)];
        if (index < 0 || index >= (int)[keys count]) return NULL;
        return strdup([keys[index] UTF8String]);
    }
}

const char* coreml_model_output_name(CoreMLModel model, int index) {
    @autoreleasepool {
        MLModel* m = (__bridge MLModel*)model;
        NSArray* keys = [[[m modelDescription].outputDescriptionsByName allKeys] sortedArrayUsingSelector:@selector(compare:)];
        if (index < 0 || index >= (int)[keys count]) return NULL;
        return strdup([keys[index] UTF8String]);
    }
}

// Helper to convert dtype enum to MLMultiArrayDataType
static MLMultiArrayDataType dtype_to_ml(int dtype) {
    switch (dtype) {
        case COREML_DTYPE_FLOAT32: return MLMultiArrayDataTypeFloat32;
        case COREML_DTYPE_FLOAT16: return MLMultiArrayDataTypeFloat16;
        case COREML_DTYPE_INT32: return MLMultiArrayDataTypeInt32;
        default: return MLMultiArrayDataTypeFloat32;
    }
}

CoreMLTensor coreml_tensor_create(int64_t* shape, int rank, int dtype, CoreMLError* error) {
    @autoreleasepool {
        NSMutableArray<NSNumber*>* dims = [NSMutableArray arrayWithCapacity:rank];
        for (int i = 0; i < rank; i++) {
            [dims addObject:@(shape[i])];
        }

        NSError* nsError = nil;
        MLMultiArray* array = [[MLMultiArray alloc] initWithShape:dims
                                                         dataType:dtype_to_ml(dtype)
                                                            error:&nsError];
        if (array == nil) {
            set_error(error, 1, nsError);
            return NULL;
        }

        return (__bridge_retained void*)array;
    }
}

CoreMLTensor coreml_tensor_create_with_data(int64_t* shape, int rank, int dtype, void* data, CoreMLError* error) {
    CoreMLTensor tensor = coreml_tensor_create(shape, rank, dtype, error);
    if (tensor == NULL) return NULL;

    @autoreleasepool {
        MLMultiArray* array = (__bridge MLMultiArray*)tensor;
        int64_t total = 1;
        for (int i = 0; i < rank; i++) {
            total *= shape[i];
        }

        // Copy data
        size_t elemSize = (dtype == COREML_DTYPE_FLOAT16) ? 2 : 4;
        if (total > 0) {
            memcpy(array.dataPointer, data, total * elemSize);
        }
    }

    return tensor;
}

void coreml_tensor_free(CoreMLTensor tensor) {
    if (tensor != NULL) {
        MLMultiArray* a = (__bridge_transfer MLMultiArray*)tensor;
        (void)a; // ARC will release
    }
}

int coreml_tensor_rank(CoreMLTensor tensor) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        return (int)[a.shape count];
    }
}

int64_t coreml_tensor_dim(CoreMLTensor tensor, int axis) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        if (axis < 0 || axis >= (int)[a.shape count]) return 0;
        return [a.shape[axis] longLongValue];
    }
}

int64_t coreml_tensor_stride(CoreMLTensor tensor, int axis) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        if (axis < 0 || axis >= (int)[a.strides count]) return 0;
        return [a.strides[axis] longLongValue];
    }
}

bool coreml_tensor_is_contiguous(CoreMLTensor tensor) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        // Check if strides are contiguous (row-major: last stride = 1, each prior = product of following dims)
        int rank = (int)[a.shape count];
        if (rank == 0) return true;

        int64_t expected = 1;
        for (int i = rank - 1; i >= 0; i--) {
            int64_t stride = [a.strides[i] longLongValue];
            if (stride != expected) return false;
            expected *= [a.shape[i] longLongValue];
        }
        return true;
    }
}

int coreml_tensor_dtype(CoreMLTensor tensor) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        switch (a.dataType) {
            case MLMultiArrayDataTypeFloat32: return COREML_DTYPE_FLOAT32;
            case MLMultiArrayDataTypeFloat16: return COREML_DTYPE_FLOAT16;
            case MLMultiArrayDataTypeInt32: return COREML_DTYPE_INT32;
            default: return COREML_DTYPE_FLOAT32;
        }
    }
}

void* coreml_tensor_data(CoreMLTensor tensor) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        return a.dataPointer;
    }
}

int64_t coreml_tensor_size_bytes(CoreMLTensor tensor) {
    @autoreleasepool {
        MLMultiArray* a = (__bridge MLMultiArray*)tensor;
        int64_t total = 1;
        for (NSNumber* dim in a.shape) {
            total *= [dim longLongValue];
        }
        switch (a.dataType) {
            case MLMultiArrayDataTypeFloat16: return total * 2;
            case MLMultiArrayDataTypeFloat32:
            case MLMultiArrayDataTypeInt32: return total * 4;
            default: return total * 4;
        }
    }
}

bool coreml_model_predict(CoreMLModel model,
                          const char** input_names, CoreMLTensor* inputs, int num_inputs,
                          const char** output_names, CoreMLTensor* outputs, int num_outputs,
                          CoreMLError* error) {
    @autoreleasepool {
        MLModel* m = (__bridge MLModel*)model;

        // Build input dictionary
        NSMutableDictionary<NSString*, MLFeatureValue*>* inputDict = [NSMutableDictionary dictionary];
        for (int i = 0; i < num_inputs; i++) {
            NSString* name = [NSString stringWithUTF8String:input_names[i]];
            MLMultiArray* array = (__bridge MLMultiArray*)inputs[i];
            MLFeatureValue* value = [MLFeatureValue featureValueWithMultiArray:array];
            inputDict[name] = value;
        }

        // Create input provider
        NSError* nsError = nil;
        MLDictionaryFeatureProvider* provider = [[MLDictionaryFeatureProvider alloc] initWithDictionary:inputDict error:&nsError];
        if (provider == nil) {
            set_error(error, 1, nsError);
            return false;
        }

        // Run prediction
        id<MLFeatureProvider> result = [m predictionFromFeatures:provider error:&nsError];
        if (result == nil) {
            set_error(error, 2, nsError);
            return false;
        }

        // Extract outputs
        for (int i = 0; i < num_outputs; i++) {
            NSString* name = [NSString stringWithUTF8String:output_names[i]];
            MLFeatureValue* value = [result featureValueForName:name];
            if (value == nil || value.multiArrayValue == nil) {
                set_error(error, 3, nil);
                return false;
            }

            // Copy output data to provided tensor — stride-aware for non-contiguous MLMultiArray outputs
            MLMultiArray* outArray = (__bridge MLMultiArray*)outputs[i];
            MLMultiArray* resultArray = value.multiArrayValue;

            int rank = (int)[resultArray.shape count];
            int64_t total = 1;
            int64_t shape[rank > 0 ? rank : 1];
            for (int d = 0; d < rank; d++) {
                shape[d] = [resultArray.shape[d] longLongValue];
                total *= shape[d];
            }

            size_t elemSize = (resultArray.dataType == MLMultiArrayDataTypeFloat16) ? 2 : 4;
            if (total > 0) {
                // Check if result array is contiguous (row-major)
                bool contiguous = true;
                int64_t expected = 1;
                for (int d = rank - 1; d >= 0; d--) {
                    int64_t stride = [resultArray.strides[d] longLongValue];
                    if (stride != expected) {
                        contiguous = false;
                        break;
                    }
                    expected *= shape[d];
                }

                if (contiguous) {
                    memcpy(outArray.dataPointer, resultArray.dataPointer, total * elemSize);
                } else {
                    // Stride-aware element-by-element copy
                    const uint8_t* src = (const uint8_t*)resultArray.dataPointer;
                    uint8_t* dst = (uint8_t*)outArray.dataPointer;

                    int64_t srcStrides[rank];
                    for (int d = 0; d < rank; d++) {
                        srcStrides[d] = [resultArray.strides[d] longLongValue];
                    }

                    int64_t indices[rank > 0 ? rank : 1];
                    memset(indices, 0, sizeof(indices));

                    for (int64_t flat = 0; flat < total; flat++) {
                        int64_t srcOffset = 0;
                        for (int d = 0; d < rank; d++) {
                            srcOffset += indices[d] * srcStrides[d];
                        }

                        memcpy(dst + flat * elemSize, src + srcOffset * elemSize, elemSize);

                        for (int d = rank - 1; d >= 0; d--) {
                            indices[d]++;
                            if (indices[d] < shape[d]) break;
                            indices[d] = 0;
                        }
                    }
                }
            }
        }

        return true;
    }
}

bool coreml_model_predict_alloc(CoreMLModel model,
                                const char** input_names, CoreMLTensor* inputs, int num_inputs,
                                char*** output_names_out, CoreMLTensor** outputs_out, int* num_outputs_out,
                                CoreMLError* error) {
    @autoreleasepool {
        MLModel* m = (__bridge MLModel*)model;

        // Build input dictionary
        NSMutableDictionary<NSString*, MLFeatureValue*>* inputDict = [NSMutableDictionary dictionary];
        for (int i = 0; i < num_inputs; i++) {
            NSString* name = [NSString stringWithUTF8String:input_names[i]];
            MLMultiArray* array = (__bridge MLMultiArray*)inputs[i];
            MLFeatureValue* value = [MLFeatureValue featureValueWithMultiArray:array];
            inputDict[name] = value;
        }

        // Create input provider
        NSError* nsError = nil;
        MLDictionaryFeatureProvider* provider = [[MLDictionaryFeatureProvider alloc] initWithDictionary:inputDict error:&nsError];
        if (provider == nil) {
            set_error(error, 1, nsError);
            return false;
        }

        // Run prediction
        id<MLFeatureProvider> result = [m predictionFromFeatures:provider error:&nsError];
        if (result == nil) {
            set_error(error, 2, nsError);
            return false;
        }

        // Collect output names sorted for deterministic order
        NSArray<NSString*>* names = [[[result featureNames] allObjects] sortedArrayUsingSelector:@selector(compare:)];
        int numOutputs = (int)[names count];
        *num_outputs_out = numOutputs;

        // Allocate arrays for output names and tensors
        *output_names_out = (char**)malloc(numOutputs * sizeof(char*));
        *outputs_out = (CoreMLTensor*)malloc(numOutputs * sizeof(CoreMLTensor));

        for (int i = 0; i < numOutputs; i++) {
            NSString* name = names[i];
            (*output_names_out)[i] = strdup([name UTF8String]);

            MLFeatureValue* value = [result featureValueForName:name];
            if (value == nil || value.multiArrayValue == nil) {
                // Clean up already allocated entries
                for (int j = 0; j < i; j++) {
                    free((*output_names_out)[j]);
                    coreml_tensor_free((*outputs_out)[j]);
                }
                free(*output_names_out);
                free(*outputs_out);
                *output_names_out = NULL;
                *outputs_out = NULL;
                *num_outputs_out = 0;
                set_error(error, 3, nil);
                return false;
            }

            MLMultiArray* resultArray = value.multiArrayValue;

            // Create a new tensor with the result's actual shape
            int rank = (int)[resultArray.shape count];
            int64_t shape[rank];
            int64_t total = 1;
            for (int d = 0; d < rank; d++) {
                shape[d] = [resultArray.shape[d] longLongValue];
                total *= shape[d];
            }

            // Convert MLMultiArrayDataType to our dtype enum
            int dtype;
            switch (resultArray.dataType) {
                case MLMultiArrayDataTypeFloat16: dtype = COREML_DTYPE_FLOAT16; break;
                case MLMultiArrayDataTypeFloat32: dtype = COREML_DTYPE_FLOAT32; break;
                case MLMultiArrayDataTypeInt32:   dtype = COREML_DTYPE_INT32; break;
                default: dtype = COREML_DTYPE_FLOAT32; break;
            }

            CoreMLError tensorError = {0, NULL};
            CoreMLTensor tensor = coreml_tensor_create(shape, rank, dtype, &tensorError);
            if (tensor == NULL) {
                for (int j = 0; j < i; j++) {
                    free((*output_names_out)[j]);
                    coreml_tensor_free((*outputs_out)[j]);
                }
                free((*output_names_out)[i]);
                free(*output_names_out);
                free(*outputs_out);
                *output_names_out = NULL;
                *outputs_out = NULL;
                *num_outputs_out = 0;
                if (tensorError.message) {
                    set_error(error, 4, nil);
                    error->message = tensorError.message;
                } else {
                    set_error(error, 4, nil);
                }
                return false;
            }

            // Copy data — stride-aware for non-contiguous MLMultiArray outputs.
            // CoreML/ANE may return arrays with non-trivial strides, so we can't just memcpy.
            MLMultiArray* outArray = (__bridge MLMultiArray*)tensor;
            size_t elemSize = (resultArray.dataType == MLMultiArrayDataTypeFloat16) ? 2 : 4;
            if (total > 0) {
                // Check if result array is contiguous (row-major)
                bool contiguous = true;
                int64_t expected = 1;
                for (int d = rank - 1; d >= 0; d--) {
                    int64_t stride = [resultArray.strides[d] longLongValue];
                    if (stride != expected) {
                        contiguous = false;
                        break;
                    }
                    expected *= shape[d];
                }

                if (contiguous) {
                    memcpy(outArray.dataPointer, resultArray.dataPointer, total * elemSize);
                } else {
                    // Stride-aware element-by-element copy
                    const uint8_t* src = (const uint8_t*)resultArray.dataPointer;
                    uint8_t* dst = (uint8_t*)outArray.dataPointer;

                    // Get strides for the result array
                    int64_t srcStrides[rank];
                    for (int d = 0; d < rank; d++) {
                        srcStrides[d] = [resultArray.strides[d] longLongValue];
                    }

                    // Iterate through all elements using multi-dimensional indices
                    // and compute source offset using strides, dest offset using row-major layout
                    int64_t indices[rank];
                    memset(indices, 0, sizeof(indices));

                    for (int64_t flat = 0; flat < total; flat++) {
                        // Compute source offset from strides
                        int64_t srcOffset = 0;
                        for (int d = 0; d < rank; d++) {
                            srcOffset += indices[d] * srcStrides[d];
                        }

                        memcpy(dst + flat * elemSize, src + srcOffset * elemSize, elemSize);

                        // Increment multi-dimensional index (last dimension first)
                        for (int d = rank - 1; d >= 0; d--) {
                            indices[d]++;
                            if (indices[d] < shape[d]) break;
                            indices[d] = 0;
                        }
                    }
                }
            }

            (*outputs_out)[i] = tensor;
        }

        return true;
    }
}
