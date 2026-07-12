// NovelIDE — an IDE for writing novels.
// Copyright (C) 2026 Matthew Kilgore
//
// This program is free software: you can redistribute it and/or modify it
// under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or (at your
// option) any later version.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of MERCHANTABILITY
// or FITNESS FOR A PARTICULAR PURPOSE. See the GNU Affero General Public
// License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package main

import (
	"embed"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is stamped from the release tag at build time: the release workflow
// (and the vendored-tarball script) write the tag into version.txt before
// building, so packaged builds report their real version while source/dev
// builds fall back to "dev".
//
//go:embed version.txt
var versionRaw string

// Version returns the app's version string ("dev" for unstamped builds).
func Version() string {
	if v := strings.TrimSpace(versionRaw); v != "" {
		return v
	}
	return "dev"
}

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "NovelIDE",
		Width:  1400,
		Height: 900,
		// Wails' Linux backend always applies a GTK max-size geometry hint,
		// and when MaxWidth/MaxHeight are 0 it caps the window to the current
		// monitor's size — so the window can't be resized beyond one screen.
		// Set an effectively-unlimited max to lift that cap. 1,000,000 px is
		// ~195x the long axis of a 5120-wide ultrawide and dwarfs any realistic
		// multi-monitor wall, while staying far below the C int (int32, ~2.1e9)
		// limit that the Wayland decorator adjustment (decorator + max) adds to.
		// A sensible floor keeps the layout from collapsing.
		MinWidth:  900,
		MinHeight: 600,
		MaxWidth:  1000000,
		MaxHeight: 1000000,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		// Needed for right-click spelling suggestions in release builds
		// (Wails hides the native context menu in production by default).
		EnableDefaultContextMenu: true,
		OnStartup:                app.startup,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
