package build

import (
	"testing"

	"github.com/docker/docker/client/buildkit"
	"github.com/docker/docker/testutil"
	moby_buildkit_v1 "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/util/progress/progressui"
	"golang.org/x/sync/errgroup"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/skip"
)

type testWriter struct {
	*testing.T
}

func (t *testWriter) Write(p []byte) (int, error) {
	t.Log(string(p))
	return len(p), nil
}

func TestBuildkitTracePropagation(t *testing.T) {
	skip.If(t, testEnv.DaemonInfo.OSType == "windows", "buildkit is not supported on Windows")

	ctx := testutil.StartSpan(baseContext, t)
	bc, err := client.New(ctx, "", buildkit.ClientOpts(testEnv.APIClient())...)
	assert.NilError(t, err)
	defer bc.Close()

	def, err := llb.Scratch().Marshal(ctx)
	assert.NilError(t, err)

	eg, ctx := errgroup.WithContext(ctx)
	ch := make(chan *client.SolveStatus)

	eg.Go(func() error {
		_, err := progressui.DisplaySolveStatus(ctx, "test", nil, &testWriter{t}, ch)
		return err
	})

	eg.Go(func() error {
		_, err := bc.Solve(ctx, def, client.SolveOpt{}, ch)
		return err
	})

	assert.NilError(t, eg.Wait())

	sub, err := bc.ControlClient().ListenBuildHistory(ctx, &moby_buildkit_v1.BuildHistoryRequest{
		EarlyExit: true,
	})
	assert.NilError(t, err)

	he, err := sub.Recv()
	assert.NilError(t, err)
	t.Log(he)
}
