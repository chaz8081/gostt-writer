module github.com/chaz8081/gostt-writer

go 1.25.6

require (
	github.com/gen2brain/malgo v0.11.24
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0-00010101000000-000000000000
	github.com/go-audio/wav v1.1.0
	github.com/robotn/gohook v0.42.3
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/go-audio/audio v1.0.0 // indirect
	github.com/go-audio/riff v1.0.0 // indirect
	github.com/vcaesar/keycode v0.10.1 // indirect
)

replace github.com/ggerganov/whisper.cpp/bindings/go => ./third_party/whisper.cpp/bindings/go
