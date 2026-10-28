package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
void installProbe(void *window, const char *source);
void reportProbe(void *window);
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const viewportWidth = 1600
const viewportHeight = 900

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 3 || os.Getenv("BISTRO_ADR_RENDERER") == "" {
		return fmt.Errorf("usage: BISTRO_ADR_RENDERER=<built-renderer> wails-spike <http://localhost:PORT/ or asset> <settings.json>")
	}
	target := os.Args[1]
	if target == "asset" {
		target = "/"
	} else if parsed, err := url.Parse(target); err != nil || parsed.Scheme != "http" || parsed.Hostname() != "localhost" {
		return fmt.Errorf("measurement URL must use http://localhost")
	}
	settings, err := os.ReadFile(os.Args[2])
	if err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if !json.Valid(settings) || len(settings) == 0 || settings[0] != '{' {
		return fmt.Errorf("settings must be a JSON object")
	}
	assets := http.FileServer(http.Dir(os.Getenv("BISTRO_ADR_RENDERER")))
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/bootstrap.html" {
			response.Header().Set("Content-Type", "text/html")
			fmt.Fprint(response, "<!doctype html><title>Benchmark bootstrap</title>")
			return
		}
		assets.ServeHTTP(response, request)
	})
	app := application.New(application.Options{
		Name: "Bistro ADR Wails", Assets: application.AssetOptions{Handler: handler},
	})
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.EditMenu)
	app.Menu.Set(menu)
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "Bistro ADR 0001 — Wails", Width: viewportWidth, Height: viewportHeight,
		MinWidth: viewportWidth, MaxWidth: viewportWidth, MinHeight: viewportHeight, MaxHeight: viewportHeight,
		URL: "/bootstrap.html", DevToolsEnabled: true,
	})
	// Wails' JS option runs after navigation. Install a document-start script before the real renderer.
	isBootstrapped := false
	window.OnWindowEvent(events.Mac.WebViewDidFinishNavigation, func(_ *application.WindowEvent) {
		application.InvokeAsync(func() {
			if !isBootstrapped {
				isBootstrapped = true
				install(window, settings)
				window.SetURL(target)
				return
			}
			C.reportProbe(window.NativeWindow())
		})
	})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		app.Quit()
	}()
	err = app.Run()
	stop()
	<-shutdownDone
	return err
}

func install(window *application.WebviewWindow, settings []byte) {
	source := C.CString("localStorage.setItem('bistro-settings-v1', JSON.stringify(" + string(settings) + "));")
	defer C.free(unsafe.Pointer(source))
	C.installProbe(window.NativeWindow(), source)
}
