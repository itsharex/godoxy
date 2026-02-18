package middleware

import (
	"net/http"

	"github.com/yusing/godoxy/internal/route/rules"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

var staticAssetsPaths = map[string]struct{}{
	// Web app manifests
	"/manifest.json":        {},
	"/manifest.webmanifest": {},
	// Service workers
	"/sw.js":         {},
	"/registerSW.js": {},
	// Favicons
	"/favicon.ico": {},
	"/favicon.png": {},
	"/favicon.svg": {},
	// Apple icons
	"/apple-icon.png":                   {},
	"/apple-touch-icon.png":             {},
	"/apple-touch-icon-precomposed.png": {},
	// Microsoft / browser config
	"/browserconfig.xml": {},
	// Safari pinned tab
	"/safari-pinned-tab.svg": {},
	// Crawlers / SEO
	"/robots.txt":        {},
	"/sitemap.xml":       {},
	"/sitemap_index.xml": {},
	"/ads.txt":           {},
}

var staticAssetsGlobs = []rules.Matcher{
	// Workbox (PWA)
	must(rules.GlobMatcher("/workbox-window.prod.es5-*.js", false)),
	must(rules.GlobMatcher("/workbox-*.js", false)),
	// Favicon variants (e.g. favicon-32x32.png)
	must(rules.GlobMatcher("/favicon-*.png", false)),
	// Web app manifest icons
	must(rules.GlobMatcher("/web-app-manifest-*.png", false)),
	// Android Chrome icons
	must(rules.GlobMatcher("/android-chrome-*.png", false)),
	// Apple touch icon variants
	must(rules.GlobMatcher("/apple-touch-icon-*.png", false)),
	// Microsoft tile icons
	must(rules.GlobMatcher("/mstile-*.png", false)),
	// Generic PWA / app icons
	must(rules.GlobMatcher("/pwa-*.png", false)),
	must(rules.GlobMatcher("/icon-*.png", false)),
	// Sitemaps (e.g. sitemap-1.xml, sitemap-posts.xml)
	must(rules.GlobMatcher("/sitemap-*.xml", false)),
}

func isStaticAssetPath(r *http.Request) bool {
	if _, ok := staticAssetsPaths[r.URL.Path]; ok {
		return true
	}

	for _, matcher := range staticAssetsGlobs {
		if matcher(r.URL.Path) {
			return true
		}
	}
	return false
}
