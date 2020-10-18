/*
Copyright © 2019 blacktop

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"fmt"
	"math"
	"os"
	"text/tabwriter"

	"github.com/AlecAivazis/survey/v2"
	"github.com/apex/log"
	"github.com/blacktop/go-macho"
	"github.com/blacktop/go-macho/pkg/trie"
	"github.com/fullsailor/pkcs7"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(machoCmd)

	machoCmd.Flags().BoolP("header", "d", false, "Print the mach header")
	machoCmd.Flags().BoolP("loads", "l", false, "Print the load commands")
	machoCmd.Flags().BoolP("sig", "s", false, "Print code signature")
	machoCmd.Flags().BoolP("ent", "e", false, "Print entitlements")
	machoCmd.Flags().BoolP("objc", "o", false, "Print ObjC info")
	machoCmd.Flags().BoolP("symbols", "n", false, "Print symbols")
	machoCmd.Flags().BoolP("starts", "f", false, "Print function starts")
	machoCmd.Flags().BoolP("fixups", "x", false, "Print fixup chains")
	machoCmd.MarkZshCompPositionalArgumentFile(1)
}

// machoCmd represents the macho command
var machoCmd = &cobra.Command{
	Use:   "macho <macho_file>",
	Short: "Parse a MachO file",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {

		var m *macho.File
		var err error

		if Verbose {
			log.SetLevel(log.DebugLevel)
		}

		showHeader, _ := cmd.Flags().GetBool("header")
		showLoadCommands, _ := cmd.Flags().GetBool("loads")
		showSignature, _ := cmd.Flags().GetBool("sig")
		showEntitlements, _ := cmd.Flags().GetBool("ent")
		showObjC, _ := cmd.Flags().GetBool("objc")
		symbols, _ := cmd.Flags().GetBool("symbols")
		showFuncStarts, _ := cmd.Flags().GetBool("starts")
		showFixups, _ := cmd.Flags().GetBool("fixups")

		onlySig := !showHeader && !showLoadCommands && showSignature && !showEntitlements && !showObjC && !showFixups && !showFuncStarts
		onlyEnt := !showHeader && !showLoadCommands && !showSignature && showEntitlements && !showObjC && !showFixups && !showFuncStarts
		onlyFixups := !showHeader && !showLoadCommands && !showSignature && !showEntitlements && !showObjC && showFixups && !showFuncStarts
		onlyFuncStarts := !showHeader && !showLoadCommands && !showSignature && !showEntitlements && !showObjC && !showFixups && showFuncStarts

		if _, err := os.Stat(args[0]); os.IsNotExist(err) {
			return fmt.Errorf("file %s does not exist", args[0])
		}

		// first check for fat file
		fat, err := macho.OpenFat(args[0])
		if err != nil && err != macho.ErrNotFat {
			return err
		}
		if err == macho.ErrNotFat {
			m, err = macho.Open(args[0])
			if err != nil {
				return err
			}
		} else {
			var options []string
			for _, arch := range fat.Arches {
				options = append(options, fmt.Sprintf("%s, %s", arch.CPU, arch.SubCPU.String(arch.CPU)))
			}

			choice := 0
			prompt := &survey.Select{
				Message: fmt.Sprintf("Detected a fat MachO file, please select an architecture to analyze:"),
				Options: options,
			}
			survey.AskOne(prompt, &choice)

			m = fat.Arches[choice].File
		}

		if showHeader && !showLoadCommands {
			fmt.Println(m.FileHeader.String())
		}
		if showLoadCommands || (!showHeader && !showLoadCommands && !showSignature && !showEntitlements && !showObjC && !showFixups && !showFuncStarts) {
			fmt.Println(m.FileTOC.String())
		}

		if showSignature {
			if !onlySig {
				fmt.Println("Code Signature")
				fmt.Println("==============")
			}
			if m.CodeSignature() != nil {
				cds := m.CodeSignature().CodeDirectories
				if len(cds) > 0 {
					for _, cd := range cds {
						fmt.Printf("Code Directory (%d bytes)\n", cd.Header.Length)
						fmt.Printf("\tVersion:     %s\n"+
							"\tFlags:       %s\n"+
							"\tCodeLimit:   0x%x\n"+
							"\tIdentifier:  %s (@0x%x)\n"+
							"\tTeamID:      %s\n"+
							"\tCDHash:      %s (computed)\n"+
							"\t# of hashes: %d code (%d pages) + %d special\n"+
							"\tHashes @%d size: %d Type: %s\n",
							cd.Header.Version,
							cd.Header.Flags,
							cd.Header.CodeLimit,
							cd.ID,
							cd.Header.IdentOffset,
							cd.TeamID,
							cd.CDHash,
							cd.Header.NCodeSlots,
							int(math.Pow(2, float64(cd.Header.PageSize))),
							cd.Header.NSpecialSlots,
							cd.Header.HashOffset,
							cd.Header.HashSize,
							cd.Header.HashType)
						if Verbose {
							for _, sslot := range cd.SpecialSlots {
								fmt.Printf("\t\t%s\n", sslot.Desc)
							}
							for _, cslot := range cd.CodeSlots {
								fmt.Printf("\t\t%s\n", cslot.Desc)
							}
						}
					}
				}
				reqs := m.CodeSignature().Requirements
				if len(reqs) > 0 {
					fmt.Printf("Requirement Set (%d bytes) with %d requirement\n",
						reqs[0].Length, // TODO: fix this (needs to be length - sizeof(header))
						len(reqs))
					for idx, req := range reqs {
						fmt.Printf("\t%d: %s (@%d, %d bytes): %s\n",
							idx,
							req.Type,
							req.Offset,
							req.Length,
							req.Detail)
					}
				}
				if len(m.CodeSignature().CMSSignature) > 0 {
					fmt.Println("CMS (RFC3852) signature:")
					p7, err := pkcs7.Parse(m.CodeSignature().CMSSignature)
					if err != nil {
						return err
					}
					w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', 0)
					for _, cert := range p7.Certificates {
						var ou string
						if cert.Issuer.Organization != nil {
							ou = cert.Issuer.Organization[0]
						}
						if cert.Issuer.OrganizationalUnit != nil {
							ou = cert.Issuer.OrganizationalUnit[0]
						}
						fmt.Fprintf(w, "        OU: %s\tCN: %s\t(%s thru %s)\n",
							ou,
							cert.Subject.CommonName,
							cert.NotBefore.Format("2006-01-02"),
							cert.NotAfter.Format("2006-01-02"))
					}
					w.Flush()
				}
			} else {
				fmt.Println("  - no code signature data")
			}
			fmt.Println()
		}

		if showEntitlements {
			if !onlyEnt {
				fmt.Println("Entitlements")
				fmt.Println("============")
			}
			if m.CodeSignature() != nil && len(m.CodeSignature().Entitlements) > 0 {
				fmt.Println(m.CodeSignature().Entitlements)
			} else {
				fmt.Println("  - no entitlements")
			}
		}

		if showObjC {
			fmt.Println("Objective-C")
			fmt.Println("===========")
			if m.HasObjC() {
				// fmt.Println("HasPlusLoadMethod: ", m.HasPlusLoadMethod())
				// fmt.Printf("GetObjCInfo: %#v\n", m.GetObjCInfo())

				// info, _ := m.GetObjCImageInfo()
				// fmt.Println(info.Flags)
				// fmt.Println(info.Flags.SwiftVersion())

				if protos, err := m.GetObjCProtocols(); err == nil {
					for _, proto := range protos {
						fmt.Println(proto.String())
					}
				}
				if classes, err := m.GetObjCClasses(); err == nil {
					for _, class := range classes {
						fmt.Println(class.String())
					}
				}
				if nlclasses, err := m.GetObjCPlusLoadClasses(); err == nil {
					for _, class := range nlclasses {
						fmt.Println(class.String())
					}
				}
				if cats, err := m.GetObjCCategories(); err == nil {
					fmt.Printf("Categories: %#v\n", cats)
				}
				if selRefs, err := m.GetObjCSelectorReferences(); err == nil {
					fmt.Println("@selectors")
					for vmaddr, name := range selRefs {
						fmt.Printf("0x%011x: %s\n", vmaddr, name)
					}
				}
				if methods, err := m.GetObjCMethodNames(); err == nil {
					fmt.Printf("\n@methods\n")
					for method, vmaddr := range methods {
						fmt.Printf("0x%011x: %s\n", vmaddr, method)
					}
				}
			} else {
				fmt.Println("  - no objc")
			}
			fmt.Println()
		}

		if showFuncStarts {
			if !onlyFuncStarts {
				fmt.Println("FUNCTION STARTS")
				fmt.Println("===============")
			}
			if m.FunctionStarts() != nil {
				for _, vaddr := range m.FunctionStarts() {
					fmt.Printf("0x%016X\n", vaddr)
				}
			}
		}

		if symbols {
			fmt.Println("SYMBOLS")
			fmt.Println("=======")
			var sec string
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
			for _, sym := range m.Symtab.Syms {
				if sym.Sect > 0 && int(sym.Sect) <= len(m.Sections) {
					sec = fmt.Sprintf("%s.%s", m.Sections[sym.Sect-1].Seg, m.Sections[sym.Sect-1].Name)
				}
				fmt.Fprintf(w, "%#016X:  <%s> \t %s\n", sym.Value, sym.Type.String(sec), sym.Name)
				// fmt.Printf("0x%016X <%s> %s\n", sym.Value, sym.Type.String(sec), sym.Name)
			}
			w.Flush()
			// Dedup these symbols (has repeats but also additional symbols??)
			if m.DyldExportsTrie() != nil && m.DyldExportsTrie().Size > 0 {
				fmt.Println("DyldExport SYMBOLS")
				fmt.Println("------------------")
				exports, err := trie.ParseTrie(m.DyldExportsTrie().Data, m.GetBaseAddress())
				if err != nil {
					return err
				}
				for _, export := range exports {
					fmt.Fprintf(w, "%#016X:  <%s> \t %s\n", export.Address, export.Flags, export.Name)
				}
				w.Flush()
			}
			if cfstrs, err := m.GetCFStrings(); err == nil {
				fmt.Println("CFStrings")
				fmt.Println("---------")
				for _, cfstr := range cfstrs {
					fmt.Printf("%#016X:  %#v\n", cfstr.Address, cfstr.Name)
				}
			}
		}

		if showFixups {
			if !onlyFixups {
				fmt.Println("FIXUPS")
				fmt.Println("======")
			}
			if m.HasFixups() {

				dcf, err := m.DyldChainedFixups()
				if err != nil {
					return err
				}

				for _, start := range dcf.Starts {
					if start.PageStarts != nil {
						if len(start.Binds) > 0 {
							fmt.Printf("\nBINDS\n")
							fmt.Println("-----")
							w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, ' ', tabwriter.Debug)
							for _, bind := range start.Binds {
								var addend string
								if fullAddend := dcf.Imports[bind.Ordinal()].Addend() + bind.Addend(); fullAddend > 0 {
									addend = fmt.Sprintf(" + 0x%x", fullAddend)
								}
								lib := m.LibraryOrdinalName(dcf.Imports[bind.Ordinal()].LibOrdinal())
								fmt.Fprintf(w, "%s\t%s/%s%s\n", bind, lib, dcf.Imports[bind.Ordinal()].Name, addend)
							}
							w.Flush()
						}
						if len(start.Rebases) > 0 {
							fmt.Printf("\nREBASES\n")
							fmt.Println("-------")
							var lastSec *macho.Section
							for _, rebase := range start.Rebases {
								addr := uint64(rebase.Offset()) + m.GetBaseAddress()
								sec := m.FindSectionForVMAddr(addr)
								if sec != lastSec {
									fmt.Printf("%s.%s\n", sec.Seg, sec.Name)
								}
								fmt.Println(rebase)
								lastSec = sec
							}
						}
					}
				}
			} else {
				fmt.Println("  - no fixups")
			}
		}

		return nil
	},
}
