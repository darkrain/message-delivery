package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBrandAssetsAndRejectInvalidIdentity(t *testing.T) {
	raw, err := os.ReadFile("../../message-delivery.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err = json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	base, err := filepath.Abs("../../templates")
	if err != nil {
		t.Fatal(err)
	}
	cfg["Templates"].(map[string]any)["BaseDir"] = base
	adapter := cfg["Providers"].(map[string]any)["Email"].(map[string]any)["Adapters"].(map[string]any)["fake-email"].(map[string]any)
	brand := map[string]any{"From": "IAMFREE <no-reply@iamfree24.com>", "HTMLLayoutFile": "email/brands/iamfree.html", "LogoFile": "email/brands/iamfree.png"}
	adapter["DefaultBrand"] = "iamfree"
	adapter["Brands"] = map[string]any{"iamfree": brand}
	path := filepath.Join(t.TempDir(), "config.json")
	load := func() (*Config, error) {
		data, _ := json.Marshal(cfg)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return Load(path)
	}
	loaded, err := load()
	if err != nil {
		t.Fatal(err)
	}
	resolved := loaded.Providers.Email.Adapters["fake-email"]["Brands"].(map[string]any)["iamfree"].(map[string]any)
	if !filepath.IsAbs(resolved["LogoFile"].(string)) || resolved["HTMLLayout"] == "" {
		t.Fatal("unresolved brand assets")
	}
	adapter["DefaultBrand"] = "missing"
	if _, err = load(); err == nil {
		t.Fatal("accepted missing default")
	}
	adapter["DefaultBrand"] = "iamfree"
	brand["From"] = "invalid"
	if _, err = load(); err == nil {
		t.Fatal("accepted invalid sender")
	}
	brand["From"] = "no-reply@iamfree24.com"
	brand["LogoFile"] = "missing.png"
	if _, err = load(); err == nil {
		t.Fatal("accepted missing logo")
	}
}
