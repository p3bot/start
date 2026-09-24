package cli

import (
	"github.com/p3bot/start/internal/orchestration"
	"github.com/spf13/cobra"
)

// resumeBareSentinel is what pflag passes to Set for a bare --resume.
// The stand-in has to be non-empty so the next argv word stays positional,
// and it has to be outside the id space: Set receives the same string for
// the bare flag and for --resume=<that string>.
const resumeBareSentinel = "\x00"

// resumeFlag records bare --resume separately from --resume=<id>.
type resumeFlag struct {
	flags *Flags
}

func (r *resumeFlag) Set(s string) error {
	r.flags.ResumeSet = true
	if s == resumeBareSentinel {
		r.flags.ResumeBare = true
		r.flags.ResumeID = ""
		return nil
	}
	r.flags.ResumeBare = false
	r.flags.ResumeID = s
	return nil
}

func (r *resumeFlag) String() string {
	if r.flags == nil || !r.flags.ResumeSet {
		return ""
	}
	if r.flags.ResumeBare {
		return ""
	}
	return r.flags.ResumeID
}

// Type is empty so help prints a bare --resume. A non-empty type is printed
// as a value name, which reads as "the next word is the id". A spaced word
// stays positional; the id is only --resume=<id>.
func (r *resumeFlag) Type() string { return "" }

// hideResumeSentinel clears the bare-flag stand-in while usage is printed.
// pflag renders NoOptDefVal in the flag line; the stand-in is not a value
// a person sets. The returned function puts it back so parsing is unchanged.
func hideResumeSentinel(root *cobra.Command) func() {
	f := root.PersistentFlags().Lookup("resume")
	if f == nil || f.NoOptDefVal == "" {
		return func() {}
	}
	saved := f.NoOptDefVal
	f.NoOptDefVal = ""
	return func() { f.NoOptDefVal = saved }
}

// installResumeUsage prints help with the bare-flag stand-in hidden, then
// restores both the stand-in and this usage function.
func installResumeUsage(root *cobra.Command) {
	var usage func(*cobra.Command) error
	usage = func(c *cobra.Command) error {
		restore := hideResumeSentinel(root)
		root.SetUsageFunc(nil)
		defer root.SetUsageFunc(usage)
		defer restore()
		return c.Usage()
	}
	root.SetUsageFunc(usage)
}

func (f *Flags) launchFlags() orchestration.LaunchFlags {
	if f == nil {
		return orchestration.LaunchFlags{}
	}
	mode := orchestration.ResumeAbsent
	id := ""
	if f.ResumeSet {
		if f.ResumeBare {
			mode = orchestration.ResumeLatest
		} else {
			mode = orchestration.ResumeID
			id = f.ResumeID
		}
	}
	return orchestration.LaunchFlags{
		Permission:    f.Permission,
		PermissionSet: f.PermissionSet,
		Effort:        f.Effort,
		EffortSet:     f.EffortSet,
		Output:        f.Output,
		OutputSet:     f.OutputSet,
		Print:         f.Print,
		Resume:        mode,
		ResumeID:      id,
	}
}
