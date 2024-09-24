package ghostscript

import (
	"bytes"
	"context"
	_ "embed"
	"testing"

	"github.com/redpanda-data/benthos/v4/public/service"

	"github.com/stretchr/testify/require"
)

var (
	//go:embed testdata/index.pdf
	pdfOne []byte
	//go:embed testdata/invoicesample.pdf
	pdfTwo []byte
)

func TestGhostscriptProcessor(t *testing.T) {
	proc, err := newGhostscriptProcessor()
	require.NoError(t, err)

	result, err := proc.Process(context.Background(), service.NewMessage(pdfOne))
	require.NoError(t, err)
	require.Len(t, result, 1)

	jpgOne, err := result[0].AsBytes()
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(jpgOne, []byte{0xff, 0xd8, 0xff, 0xe0}))

	result, err = proc.Process(context.Background(), service.NewMessage(pdfTwo))
	require.NoError(t, err)
	require.Len(t, result, 1)

	jpgTwo, err := result[0].AsBytes()
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(jpgTwo, []byte{0xff, 0xd8, 0xff, 0xe0}))
	require.NotEqual(t, jpgOne, jpgTwo)
}
