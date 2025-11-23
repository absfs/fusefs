package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/absfs/fusefs"
)

func main() {
	// Create a temporary directory with some content
	tempDir, err := os.MkdirTemp("", "fusefs-source-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create some initial content
	if err := os.Mkdir(filepath.Join(tempDir, "documents"), 0755); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "documents", "hello.txt"), []byte("Hello, FUSE!"), 0644); err != nil {
		log.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tempDir, "documents", "world.txt"), []byte("Welcome to fusefs!"), 0644); err != nil {
		log.Fatal(err)
	}

	// Create README
	readme := `# fusefs Example

This is a FUSE-mounted filesystem.

You can:
- Read files: cat documents/hello.txt
- List directories: ls -la documents/
- Create files: echo "test" > test.txt
- Create directories: mkdir mydir

Try it out!
`
	if err := os.WriteFile(filepath.Join(tempDir, "README.md"), []byte(readme), 0644); err != nil {
		log.Fatal(err)
	}

	// Create filesystem adapter
	osfs := NewOSFS(tempDir)

	// Get mountpoint from args or use default
	mountpoint := "/tmp/fusefs-basic"
	if len(os.Args) > 1 {
		mountpoint = os.Args[1]
	}

	// Mount filesystem
	opts := fusefs.DefaultMountOptions(mountpoint)
	opts.FSName = "fusefs-example"
	opts.AllowOther = false
	opts.Debug = false

	fmt.Printf("Mounting filesystem from %s at %s...\n", tempDir, mountpoint)

	fuseFS, err := fusefs.Mount(osfs, opts)
	if err != nil {
		log.Fatalf("Failed to mount: %v", err)
	}
	defer fuseFS.Unmount()

	fmt.Printf("✓ Mounted successfully at %s\n", mountpoint)
	fmt.Printf("Try:\n")
	fmt.Printf("  ls -la %s\n", mountpoint)
	fmt.Printf("  cat %s/README.md\n", mountpoint)
	fmt.Printf("  cat %s/documents/hello.txt\n", mountpoint)
	fmt.Printf("\nPress Ctrl+C to unmount...\n")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nUnmounting...")

	// Print statistics before unmounting
	stats := fuseFS.Stats()
	fmt.Printf("\nStatistics:\n")
	fmt.Printf("  Operations:    %d\n", stats.Operations)
	fmt.Printf("  Bytes Read:    %d\n", stats.BytesRead)
	fmt.Printf("  Bytes Written: %d\n", stats.BytesWritten)
	fmt.Printf("  Errors:        %d\n", stats.Errors)
	fmt.Printf("  Open Files:    %d\n", stats.OpenFiles)
}
