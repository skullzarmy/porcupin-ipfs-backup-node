package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ASCII logo - 42 chars wide, 12 lines
const logo = `
           ++   ++                
           +++   ++  ++  +            
             ++++++++++++++           
        ++++++++++++++++++++          
           +++    ++++++++++++        
     +++++++++    ++++++++++ +++      
        ++++++++++  ++++++++++++++++  
    +++++++++++++++  +++++++++++++    
        +++++++++++++  ++++++         
     +++++++++++++++++  +++           
    +   ++++++++++++++++  +           
       ++++++++++++++++++++++
`

const logoWidth = 42

// ANSI escape codes
const (
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Reset     = "\033[0m"
	Cyan      = "\033[36m"
	Green     = "\033[32m"
	Yellow    = "\033[33m"
	White     = "\033[37m"
)

// hrule returns a horizontal rule of the given width
func hrule(width int) string {
	return Dim + strings.Repeat("─", width) + Reset
}

// PrintBanner prints the ASCII logo if conditions are right
func PrintBanner() {
	if !shouldShowBanner() {
		return
	}
	fmt.Print(logo)
}

// PrintBannerWithVersion prints the logo and version info
func PrintBannerWithVersion(version string) {
	if !shouldShowBanner() {
		fmt.Printf("Porcupin %s\n", version)
		return
	}
	
	fmt.Print(logo)
	fmt.Println(hrule(logoWidth))
	
	// Center the version text
	versionText := fmt.Sprintf("%sPorcupin%s %s%s%s", Bold, Reset, Cyan, version, Reset)
	// Account for ANSI codes in padding calculation
	visibleLen := len("Porcupin ") + len(version)
	padding := (logoWidth - visibleLen) / 2
	if padding > 0 {
		fmt.Printf("%s%s\n", strings.Repeat(" ", padding), versionText)
	} else {
		fmt.Println(versionText)
	}
	
	fmt.Println(hrule(logoWidth))
	fmt.Println()
}

// PrintStats prints formatted statistics
func PrintStats(nfts, total, pinned, pending, failed int64, storageGB float64) {
	useColor := shouldShowBanner() // colors only if TTY
	
	if useColor {
		fmt.Println(hrule(logoWidth))
		fmt.Printf("%s%sPorcupin Stats%s\n", Bold, White, Reset)
		fmt.Println()
		fmt.Printf("  %sNFTs:%s          %s%d%s\n", Dim, Reset, Bold, nfts, Reset)
		fmt.Printf("  %sAssets:%s        %s%d%s\n", Dim, Reset, Bold, total, Reset)
		fmt.Printf("  %sPinned:%s        %s%s%d%s\n", Dim, Reset, Green, Bold, pinned, Reset)
		fmt.Printf("  %sPending:%s       %s%s%d%s\n", Dim, Reset, Yellow, Bold, pending, Reset)
		fmt.Printf("  %sFailed:%s        %s%d%s\n", Dim, Reset, Bold, failed, Reset)
		fmt.Printf("  %sStorage:%s       %s%.2f GB%s\n", Dim, Reset, Bold, storageGB, Reset)
		fmt.Println()
		fmt.Println(hrule(logoWidth))
	} else {
		fmt.Println("Porcupin Stats")
		fmt.Printf("  NFTs:          %d\n", nfts)
		fmt.Printf("  Assets:        %d\n", total)
		fmt.Printf("  Pinned:        %d\n", pinned)
		fmt.Printf("  Pending:       %d\n", pending)
		fmt.Printf("  Failed:        %d\n", failed)
		fmt.Printf("  Storage:       %.2f GB\n", storageGB)
	}
}

// shouldShowBanner checks if we should display the ASCII banner
func shouldShowBanner() bool {
	// Respect NO_COLOR environment variable (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// Check if stdout is a terminal
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	// Check terminal width
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return false
	}

	if width < logoWidth {
		return false
	}

	return true
}

// IsTTY returns true if stdout is a terminal
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// PrintAbout prints the about information
func PrintAbout(version string) {
	showBanner := shouldShowBanner()
	useColor := showBanner
	
	if showBanner {
		fmt.Print(logo)
		fmt.Println(hrule(logoWidth))
	}
	
	if useColor {
		fmt.Printf("            %s%sPorcupin%s %s%s%s\n", Bold, White, Reset, Cyan, version, Reset)
		fmt.Printf("        %sTezos NFT Backup to IPFS%s\n", Dim, Reset)
		fmt.Println()
		fmt.Println("  Automatically backup your Tezos NFT assets")
		fmt.Println("  to a local IPFS node. Set it and forget it –")
		fmt.Println("  Porcupin watches your wallets and pins new")
		fmt.Println("  NFTs as they arrive.")
		fmt.Println()
		fmt.Println(hrule(logoWidth))
		fmt.Println()
		fmt.Printf("  %sGitHub:%s  github.com/skullzarmy/porcupin-ipfs-backup-node\n", Dim, Reset)
		fmt.Printf("  %sIssues:%s  github.com/skullzarmy/porcupin-ipfs-backup-node/issues\n", Dim, Reset)
		fmt.Printf("  %sEmail:%s   info@fafolab.xyz\n", Dim, Reset)
		fmt.Println()
		fmt.Println(hrule(logoWidth))
		fmt.Println()
		fmt.Printf("  %s♥%s Developed by %sFAFOlab%s  %s•%s  fafolab.xyz\n", "\033[31m", Reset, Bold, Reset, Dim, Reset)
		fmt.Printf("  %s© 2025 Porcupin. MIT License.%s\n", Dim, Reset)
	} else {
		fmt.Printf("Porcupin %s\n", version)
		fmt.Println("Tezos NFT Backup to IPFS")
		fmt.Println()
		fmt.Println("Automatically backup your Tezos NFT assets to a local IPFS node.")
		fmt.Println("Set it and forget it – Porcupin watches your wallets and pins")
		fmt.Println("new NFTs as they arrive.")
		fmt.Println()
		fmt.Println("GitHub:  github.com/skullzarmy/porcupin-ipfs-backup-node")
		fmt.Println("Issues:  github.com/skullzarmy/porcupin-ipfs-backup-node/issues")
		fmt.Println("Email:   info@fafolab.xyz")
		fmt.Println()
		fmt.Println("Developed by FAFOlab  •  fafolab.xyz")
		fmt.Println("© 2025 Porcupin. MIT License.")
	}
	
	if showBanner {
		fmt.Println()
		fmt.Println(hrule(logoWidth))
	}
}
