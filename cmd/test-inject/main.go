// Command test-inject is a manual test for text injection.
// It waits 3 seconds, then types or pastes test text.
// Focus a text editor before the countdown finishes.
//
// Usage:
//
//	go run ./cmd/test-inject [--method type|paste]
package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/chaz8081/gostt-writer/internal/inject"
)

func main() {
	method := flag.String("method", "type", "inject method: type or paste")
	flag.Parse()

	text := "Hello from gostt-writer!"

	fmt.Printf("Will inject %q using %q method in 3 seconds...\n", text, *method)
	fmt.Println("Focus a text editor now!")

	for i := 3; i > 0; i-- {
		fmt.Printf("%d...\n", i)
		time.Sleep(time.Second)
	}

	inj := inject.NewInjector(*method)
	if err := inj.Inject(text); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("\nDone!")
}
