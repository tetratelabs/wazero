// Copyright (C) 2022 Print Tracker, LLC - All Rights Reserved
//
// Unauthorized copying of this file, via any medium is strictly prohibited
// as this source code is proprietary and confidential. Dissemination of this
// information or reproduction of this material is strictly forbidden unless
// prior written permission is obtained from Print Tracker, LLC.

package bindgen

import (
	"context"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"testing"
)

func TestBindgen(t *testing.T) {
	ctx := context.Background()
	r := wazero.NewRuntime()
	defer r.Close(ctx)

	bg, err := Instantiate(ctx, r)
	require.NoError(t, err)
	_ = bg
}
