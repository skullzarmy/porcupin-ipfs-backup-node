package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// =============================================================================
// PURE FUNCTION TESTS
// =============================================================================

func TestHrule(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		expected int // expected number of ─ characters
	}{
		{"zero width", 0, 0},
		{"width 1", 1, 1},
		{"width 10", 10, 10},
		{"width 42 (logo width)", 42, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hrule(tt.width)
			// Count the ─ characters (3 bytes each in UTF-8)
			count := strings.Count(result, "─")
			if count != tt.expected {
				t.Errorf("hrule(%d) has %d dashes, want %d", tt.width, count, tt.expected)
			}
			// Should contain ANSI codes
			if !strings.Contains(result, Dim) {
				t.Error("hrule should contain Dim ANSI code")
			}
			if !strings.Contains(result, Reset) {
				t.Error("hrule should contain Reset ANSI code")
			}
		})
	}
}

func TestLogoConstants(t *testing.T) {
	// Logo should be non-empty
	if len(logo) == 0 {
		t.Error("logo constant should not be empty")
	}

	// Logo width constant should be positive
	if logoWidth <= 0 {
		t.Error("logoWidth should be positive")
	}

	// Logo width should be 42 (as documented)
	if logoWidth != 42 {
		t.Errorf("logoWidth = %d, want 42", logoWidth)
	}
}

func TestANSIConstants(t *testing.T) {
	// All ANSI codes should start with escape sequence
	codes := map[string]string{
		"Bold":   Bold,
		"Dim":    Dim,
		"Reset":  Reset,
		"Cyan":   Cyan,
		"Green":  Green,
		"Yellow": Yellow,
		"White":  White,
	}

	for name, code := range codes {
		if !strings.HasPrefix(code, "\033[") {
			t.Errorf("%s should start with escape sequence, got %q", name, code)
		}
		if !strings.HasSuffix(code, "m") {
			t.Errorf("%s should end with 'm', got %q", name, code)
		}
	}
}

// =============================================================================
// OUTPUT CAPTURE TESTS
// =============================================================================

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestPrintStatsNoColor(t *testing.T) {
	// Set NO_COLOR to force plain output
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintStats(100, 500, 450, 25, 25, 54.32)
	})

	// Should contain all stat labels
	expectedLabels := []string{"NFTs:", "Assets:", "Pinned:", "Pending:", "Failed:", "Storage:"}
	for _, label := range expectedLabels {
		if !strings.Contains(output, label) {
			t.Errorf("PrintStats output should contain %q", label)
		}
	}

	// Should contain the values
	expectedValues := []string{"100", "500", "450", "25", "54.32"}
	for _, val := range expectedValues {
		if !strings.Contains(output, val) {
			t.Errorf("PrintStats output should contain %q", val)
		}
	}

	// Should NOT contain ANSI codes when NO_COLOR is set
	if strings.Contains(output, "\033[") {
		t.Error("PrintStats should not contain ANSI codes when NO_COLOR is set")
	}
}

func TestPrintBannerWithVersionNoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintBannerWithVersion("0.1.4")
	})

	// Should contain version
	if !strings.Contains(output, "0.1.4") {
		t.Error("PrintBannerWithVersion should contain version")
	}

	// Should contain Porcupin
	if !strings.Contains(output, "Porcupin") {
		t.Error("PrintBannerWithVersion should contain 'Porcupin'")
	}

	// Should NOT contain the ASCII logo when NO_COLOR (implies non-TTY behavior)
	// Just verify it's a simple output
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 5 {
		t.Error("PrintBannerWithVersion with NO_COLOR should be simple output")
	}
}

func TestPrintAboutNoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintAbout("0.1.4")
	})

	// Should contain key information
	expectedContent := []string{
		"Porcupin",
		"0.1.4",
		"Tezos NFT",
		"IPFS",
		"github.com",
		"FAFOlab",
		"MIT License",
	}

	for _, content := range expectedContent {
		if !strings.Contains(output, content) {
			t.Errorf("PrintAbout output should contain %q", content)
		}
	}

	// Should NOT contain ANSI codes
	if strings.Contains(output, "\033[") {
		t.Error("PrintAbout should not contain ANSI codes when NO_COLOR is set")
	}
}

func TestPrintBannerNoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintBanner()
	})

	// When NO_COLOR is set, banner should not print
	if output != "" {
		t.Error("PrintBanner should produce no output when NO_COLOR is set")
	}
}

// =============================================================================
// EDGE CASE TESTS
// =============================================================================

func TestPrintStatsZeroValues(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintStats(0, 0, 0, 0, 0, 0.0)
	})

	// Should still produce output with zero values
	if !strings.Contains(output, "0") {
		t.Error("PrintStats should show zero values")
	}
}

func TestPrintStatsLargeValues(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintStats(1000000, 5000000, 4500000, 250000, 250000, 1024.56)
	})

	// Should handle large numbers
	if !strings.Contains(output, "1000000") {
		t.Error("PrintStats should handle large NFT counts")
	}
	if !strings.Contains(output, "1024.56") {
		t.Error("PrintStats should handle large storage values")
	}
}

func TestPrintBannerWithVersionEmpty(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintBannerWithVersion("")
	})

	// Should still produce output even with empty version
	if !strings.Contains(output, "Porcupin") {
		t.Error("PrintBannerWithVersion should work with empty version")
	}
}

func TestPrintAboutEmpty(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	output := captureOutput(func() {
		PrintAbout("")
	})

	// Should still produce output even with empty version
	if !strings.Contains(output, "Porcupin") {
		t.Error("PrintAbout should work with empty version")
	}
}

// =============================================================================
// SHOULDSHOWBANNER TESTS
// =============================================================================

func TestShouldShowBannerWithNoColor(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")

	if shouldShowBanner() {
		t.Error("shouldShowBanner should return false when NO_COLOR is set")
	}
}

func TestShouldShowBannerRespectsNoColorValues(t *testing.T) {
	// NO_COLOR spec says any non-empty value should disable color
	testValues := []string{"1", "true", "yes", "anything"}

	for _, val := range testValues {
		os.Setenv("NO_COLOR", val)
		if shouldShowBanner() {
			t.Errorf("shouldShowBanner should return false when NO_COLOR=%q", val)
		}
		os.Unsetenv("NO_COLOR")
	}
}
