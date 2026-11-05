package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jhinw/flybrain/flybody/internal/app/runtime"
	"github.com/jhinw/flybrain/flybody/internal/domain/organ"
)

// DefaultStateFile is the default local snapshot file used across CLI invocations.
const DefaultStateFile = ".flysoul.json"

// CLIApp belongs to the Infrastructure Layer.
// Primary Responsibility: Parse user CLI commands (`inspect`, `stim`, `step`, `snapshot`, `repl`),
// assign stimulation/stepping tasks to PhysiologyRuntime, and print formatted results.
type CLIApp struct {
	rt        *runtime.PhysiologyRuntime
	stateFile string
	out       io.Writer
}

// NewCLIApp creates a new CLIApp bound to a PhysiologyRuntime.
func NewCLIApp(rt *runtime.PhysiologyRuntime, stateFile string, out io.Writer) *CLIApp {
	if stateFile == "" {
		stateFile = DefaultStateFile
	}
	if out == nil {
		out = os.Stdout
	}
	return &CLIApp{
		rt:        rt,
		stateFile: stateFile,
		out:       out,
	}
}

// Run executes a single CLI command line argument slice.
func (app *CLIApp) Run(args []string) error {
	// Parse optional global --state flag if present before the subcommand
	for len(args) >= 2 && strings.HasPrefix(args[0], "--state") {
		if strings.Contains(args[0], "=") {
			parts := strings.SplitN(args[0], "=", 2)
			app.stateFile = parts[1]
			args = args[1:]
		} else {
			app.stateFile = args[1]
			args = args[2:]
		}
	}

	// Automatically load existing state file if present on disk
	if _, err := os.Stat(app.stateFile); err == nil {
		_ = app.rt.LoadFromFile(app.stateFile)
	}

	if len(args) == 0 {
		app.printUsage()
		return nil
	}

	subcmd := strings.ToLower(args[0])
	subArgs := args[1:]

	switch subcmd {
	case "inspect", "status":
		return app.handleInspect(subArgs)
	case "stim", "stimulate":
		return app.handleStim(subArgs)
	case "step", "tick":
		return app.handleStep(subArgs)
	case "snapshot":
		return app.handleSnapshot(subArgs)
	case "reset":
		return app.handleReset()
	case "repl":
		return app.handleREPL()
	case "help", "-h", "--help":
		app.printUsage()
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (run 'flybody help' for usage)", subcmd)
	}
}

func (app *CLIApp) handleInspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	format := fs.String("format", "tree", "Output format: tree or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.ToLower(*format) == "json" {
		s, err := FormatBodyJSON(app.rt)
		if err != nil {
			return err
		}
		fmt.Fprintln(app.out, s)
		return nil
	}
	fmt.Fprint(app.out, FormatBodyTree(app.rt))
	return nil
}

func (app *CLIApp) handleStep(args []string) error {
	fs := flag.NewFlagSet("step", flag.ContinueOnError)
	ticks := fs.Int("ticks", 5, "Number of physiological ticks to advance")
	if err := fs.Parse(args); err != nil {
		return err
	}
	tr := app.rt.StepTicks(*ticks, runtime.DefaultTickDuration)
	_ = app.rt.SaveToFile(app.stateFile)
	fmt.Fprint(app.out, FormatTransition(app.rt, tr))
	return nil
}

func (app *CLIApp) handleStim(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing target organ for 'stim' (expected: eye, antenna, touch, proboscis)")
	}
	target := strings.ToLower(args[0])
	flagArgs := args[1:]

	b := app.rt.Body()
	var ticks int

	switch target {
	case "eye", "eyes":
		fs := flag.NewFlagSet("stim eye", flag.ContinueOnError)
		side := fs.String("side", "both", "Eye side: left, right, or both")
		wavelength := fs.Float64("wavelength", 0, "Light wavelength in nm (e.g. 365 for UV, 450 for Blue, 520 for Green)")
		colorHex := fs.String("color", "", "Hex RGB color (e.g. #8A2BE2 or #FF4500)")
		rgbAlias := fs.String("rgb", "", "Alias for --color")
		intensity := fs.Float64("intensity", 0.85, "Light intensity [0.0..1.0]")
		looming := fs.Float64("looming-rate", 0.0, "Looming solid-angle expansion rate [0.0..1.0]")
		azimuth := fs.Float64("azimuth", 0.0, "Horizontal angle in degrees (-90 left .. +90 right)")
		elevation := fs.Float64("elevation", 0.0, "Vertical angle in degrees")
		ticksFlag := fs.Int("ticks", 4, "Number of ticks to stimulate and advance")
		if err := fs.Parse(flagArgs); err != nil {
			return err
		}
		hex := *colorHex
		if hex == "" {
			hex = *rgbAlias
		}
		ticks = *ticksFlag
		b.Eyes.PerceiveLight(organ.VisualStimulus{
			Side:          organ.EyeSide(strings.ToLower(*side)),
			WavelengthNM:  *wavelength,
			ColorHex:      hex,
			Intensity:     *intensity,
			LoomingRate:   *looming,
			AzimuthDeg:    *azimuth,
			ElevationDeg:  *elevation,
			DurationTicks: ticks,
		})

	case "antenna", "antennae":
		fs := flag.NewFlagSet("stim antenna", flag.ContinueOnError)
		odor := fs.String("odor", "food", "Odor type: food or danger")
		intensity := fs.Float64("intensity", 0.75, "Default bilateral odor concentration [0.0..1.0]")
		left := fs.Float64("left", -1, "Left antenna odor concentration [0.0..1.0] (overrides --intensity)")
		right := fs.Float64("right", -1, "Right antenna odor concentration [0.0..1.0] (overrides --intensity)")
		wind := fs.Float64("wind", 0.0, "Wind airflow strength [0.0..1.0]")
		temp := fs.Float64("temp", -1.0, "Temperature [0.0=cold, 0.5=25C, 1.0=hot]")
		ticksFlag := fs.Int("ticks", 4, "Number of ticks to stimulate and advance")
		if err := fs.Parse(flagArgs); err != nil {
			return err
		}
		lVal := *intensity
		rVal := *intensity
		if *left >= 0 {
			lVal = *left
		}
		if *right >= 0 {
			rVal = *right
		}
		ticks = *ticksFlag
		b.Antennae.PerceiveAntennal(organ.AntennalStimulus{
			OdorType:       organ.OdorType(strings.ToLower(*odor)),
			LeftOdorConc:   lVal,
			RightOdorConc:  rVal,
			WindStrength:   *wind,
			Temperature:    *temp,
			HasTemperature: *temp >= 0,
			DurationTicks:  ticks,
		})

	case "touch", "bristle", "bristles":
		fs := flag.NewFlagSet("stim touch", flag.ContinueOnError)
		segment := fs.String("segment", "thorax", "Body segment: head, thorax, abdomen")
		organName := fs.String("organ", "", "Specific organ target (e.g. leg_r1)")
		force := fs.Float64("force", 0.65, "Mechanical touch force [0.0..1.0] (>=0.85 triggers nociception)")
		ticksFlag := fs.Int("ticks", 4, "Number of ticks to stimulate and advance")
		if err := fs.Parse(flagArgs); err != nil {
			return err
		}
		loc := *segment
		if *organName != "" {
			loc = *organName
		}
		ticks = *ticksFlag
		b.Bristles.PerceiveTouch(organ.TouchStimulus{
			TargetLocation: strings.ToLower(loc),
			Force:          *force,
			DurationTicks:  ticks,
		})

	case "proboscis", "taste", "feed":
		fs := flag.NewFlagSet("stim proboscis", flag.ContinueOnError)
		taste := fs.String("taste", "sweet", "Taste type: sweet, bitter, or water")
		conc := fs.Float64("concentration", 0.85, "Tastant concentration [0.0..1.0]")
		ticksFlag := fs.Int("ticks", 4, "Number of ticks to stimulate and advance")
		if err := fs.Parse(flagArgs); err != nil {
			return err
		}
		ticks = *ticksFlag
		b.Proboscis.PerceiveTaste(organ.GustatoryStimulus{
			Taste:         organ.TasteType(strings.ToLower(*taste)),
			Concentration: *conc,
			DurationTicks: ticks,
		})

	default:
		return fmt.Errorf("unknown organ %q for 'stim' (expected: eye, antenna, touch, proboscis)", target)
	}

	tr := app.rt.StepTicks(ticks, runtime.DefaultTickDuration)
	_ = app.rt.SaveToFile(app.stateFile)
	fmt.Fprint(app.out, FormatTransition(app.rt, tr))
	return nil
}

func (app *CLIApp) handleSnapshot(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: flybody snapshot <save|load> <filepath>")
	}
	action := strings.ToLower(args[0])
	targetPath := args[1]

	switch action {
	case "save":
		if err := app.rt.SaveToFile(targetPath); err != nil {
			return err
		}
		fmt.Fprintf(app.out, "Saved virtual body & brain snapshot (Tick #%d) to %s\n", app.rt.TickID(), targetPath)
		return nil
	case "load":
		if err := app.rt.LoadFromFile(targetPath); err != nil {
			return err
		}
		_ = app.rt.SaveToFile(app.stateFile)
		fmt.Fprintf(app.out, "Restored virtual body & brain snapshot (Tick #%d) from %s\n", app.rt.TickID(), targetPath)
		fmt.Fprint(app.out, FormatBodyTree(app.rt))
		return nil
	default:
		return fmt.Errorf("unknown snapshot action %q (expected: save or load)", action)
	}
}

func (app *CLIApp) handleReset() error {
	_ = os.Remove(app.stateFile)
	fmt.Fprintf(app.out, "Reset state file %s.\n", app.stateFile)
	return nil
}

func (app *CLIApp) handleREPL() error {
	fmt.Fprintln(app.out, "=== Drosophila Virtual Body (`flybody`) Interactive REPL ===")
	fmt.Fprintln(app.out, "Type commands without the 'flybody' prefix (e.g., 'inspect', 'stim eye --wavelength=365', 'step --ticks=5', 'exit').")
	fmt.Fprint(app.out, FormatBodyTree(app.rt))

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprintf(app.out, "\nflybody(#%d)> ", app.rt.TickID())
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			fmt.Fprintln(app.out, "Exiting REPL.")
			break
		}
		tokens := strings.Fields(line)
		if err := app.Run(tokens); err != nil {
			fmt.Fprintf(app.out, "Error: %v\n", err)
		}
	}
	return scanner.Err()
}

func (app *CLIApp) printUsage() {
	fmt.Fprint(app.out, `Drosophila Virtual Body CLI Prototype (flybody - Iteration 1.0)

Usage:
  flybody [--state=.flysoul.json] <command> [flags]

Commands:
  inspect [--format=tree|json]
      Inspect the 3 anatomical segments (HEAD, THORAX, ABDOMEN), organs, active brain regions, and 3D pose.

  stim eye [--side=left|right|both] [--wavelength=365] [--color="#8A2BE2"] [--intensity=0.8] [--looming-rate=0.0] [--ticks=4]
      Stimulate compound eyes with wavelength/RGB color, light intensity, or looming expansion.

  stim antenna [--odor=food|danger] [--left=0.35] [--right=0.75] [--wind=0.0] [--temp=0.5] [--ticks=4]
      Stimulate left/right antennae with stereo odor gradients, wind airflow, or temperature.

  stim touch [--segment=head|thorax|abdomen] [--organ=leg_r1] [--force=0.65] [--ticks=4]
      Stimulate cuticular bristles with mechanical touch (or noxious force >= 0.85).

  stim proboscis [--taste=sweet|bitter|water] [--concentration=0.85] [--ticks=4]
      Stimulate labellar taste receptors on the proboscis to trigger feeding or aversion.

  step [--ticks=5]
      Advance N natural physiological ticks and display before/after organ state changes.

  snapshot save|load <filepath>
      Export or restore the complete virtual body + brain state container (.flysoul.json).

  repl
      Launch an interactive physiological REPL session.

  reset
      Remove the current .flysoul.json state file and reset to a fresh virtual body.
`)
}
