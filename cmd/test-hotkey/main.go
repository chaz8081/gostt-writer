// Command test-hotkey is a manual test for the global hotkey listener.
// Run it, then press Ctrl+Shift+R to see events.
// Press Ctrl+C to exit.
//
// Usage:
//
//	go run ./cmd/test-hotkey [--mode hold|toggle]
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaz8081/gostt-writer/internal/hotkey"
)

func main() {
	mode := flag.String("mode", "hold", "hotkey mode: hold or toggle")
	flag.Parse()

	keys := []string{"ctrl", "shift", "r"}
	fmt.Printf("Listening for Ctrl+Shift+R in %q mode...\n", *mode)
	fmt.Println("Press Ctrl+C to exit.")

	listener := hotkey.NewListener(keys, *mode)

	// Handle Ctrl+C
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nShutting down...")
		listener.Stop()
	}()

	// Read events
	go func() {
		for ev := range listener.Events() {
			switch ev.Type {
			case hotkey.EventStart:
				fmt.Println(">>> START (recording)")
			case hotkey.EventStop:
				fmt.Println("<<< STOP  (stopped)")
			}
		}
		fmt.Println("Event channel closed.")
	}()

	// Blocks until stopped
	listener.Start()
	fmt.Println("Done.")
}
