package ghostscript

import (
	"context"
	"embed"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/redpanda-data/benthos/v4/public/service"
	"github.com/spf13/afero"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	"github.com/tetratelabs/wazero/imports/emscripten"
	wasip1 "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed wasm/gs.wasm
var gsWasm []byte

//go:embed ghostscript
var sharedFiles embed.FS

func init() {
	// Config spec is empty for now as we don't have any dynamic fields.
	configSpec := service.NewConfigSpec()

	constructor := func(conf *service.ParsedConfig, mgr *service.Resources) (service.Processor, error) {
		return newGhostscriptProcessor()
	}

	err := service.RegisterProcessor("ghostscript", configSpec, constructor)
	if err != nil {
		panic(err)
	}
}

type gsProcessor struct {
	fs       *memAdaptFS
	compiled wazero.CompiledModule
	config   wazero.ModuleConfig
	runtime  wazero.Runtime
	idGen    atomic.Int64
}

func newGhostscriptProcessor() (*gsProcessor, error) {
	ctx := context.Background()
	runtimeConfig := wazero.NewRuntimeConfig().WithCloseOnContextDone(true)
	wazeroRuntime := wazero.NewRuntimeWithConfig(ctx, runtimeConfig)

	if _, err := wasip1.Instantiate(ctx, wazeroRuntime); err != nil {
		return nil, err
	}

	compiledModule, err := wazeroRuntime.CompileModule(ctx, gsWasm)
	if err != nil {
		return nil, err
	}
	if _, err := emscripten.InstantiateForModule(ctx, wazeroRuntime, compiledModule); err != nil {
		return nil, err
	}

	fsConfig := wazero.NewFSConfig()
	mfs := NewInMemoryWasmFS()
	fsConfig = fsConfig.(sysfs.FSConfig).WithSysFSMount(mfs, "/rpcn")
	moduleConfig := wazero.NewModuleConfig().
		WithStartFunctions("_start").
		WithStdout(os.Stdout).
		WithStderr(os.Stdout).
		WithFSConfig(fsConfig).
		WithName("").      // Make anonymous! Needed for concurrent usage
		WithSysWalltime(). // Real clocks is needed to generate correct (non-duplicate) tmp file names.
		WithSysNanotime().
		WithSysNanosleep()

	return &gsProcessor{
		fs:       mfs,
		compiled: compiledModule,
		config:   moduleConfig,
		runtime:  wazeroRuntime,
	}, nil
}

func (p *gsProcessor) Process(ctx context.Context, m *service.Message) (service.MessageBatch, error) {
	id := p.idGen.Add(1)
	b, err := m.AsBytes()
	if err != nil {
		return nil, err
	}
	inputPath := fmt.Sprintf("io/input-%d.pdf", id)
	afero.WriteFile(p.fs.original, inputPath, b, 0o666)
	defer p.fs.original.Remove(inputPath)
	args := []string{
		"gs",
		"-dNOPAUSE",
		"-dBATCH",
		"-sDEVICE=jpeg",
		fmt.Sprintf("-sOutputFile=/rpcn/io/%d-output-%%02d.jpg", id),
		filepath.Join("/rpcn", inputPath),
	}
	if _, err := p.runtime.InstantiateModule(ctx, p.compiled, p.config.WithArgs(args...)); err != nil {
		return nil, err
	}
	outFiles, err := afero.Glob(p.fs.original, fmt.Sprintf("io/%d-output-*.jpg", id))
	if err != nil {
		return nil, err
	}
	batch := make(service.MessageBatch, len(outFiles))
	for i, file := range outFiles {
		img, err := afero.ReadFile(p.fs.original, file)
		if err != nil {
			return nil, err
		}
		if len(img) == 0 {
			break
		}
		msg := m.Copy()
		msg.SetBytes(img)
		batch[i] = msg
		if err := p.fs.original.Remove(file); err != nil {
			return nil, err
		}
	}
	return batch, nil
}

func (p *gsProcessor) Close(ctx context.Context) error {
	return p.runtime.Close(ctx)
}
