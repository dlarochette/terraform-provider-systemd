//go:build !skip_acc

package provider

import "testing"

// TestAccTypedUnits writes typed units and checks load/enable state.
// Timer/path/socket/swap are enabled; slices without [Install] stay
// UnitFileState=static. None are started (swap would need a real device).
func TestAccTypedUnits(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	type tc struct {
		name       string
		content    string
		enable     bool
		wantLoad   string
		wantEnable string // empty = do not assert UnitFileState
	}
	cases := []tc{
		{
			name: "tf-acc.timer",
			content: "[Unit]\n" +
				"Description=tf-acc timer\n" +
				"\n" +
				"[Timer]\n" +
				"OnUnitActiveSec=1h\n" +
				"Persistent=false\n" +
				"\n" +
				"[Install]\n" +
				"WantedBy=timers.target\n",
			enable:     true,
			wantLoad:   "loaded",
			wantEnable: "enabled",
		},
		{
			name: "tf-acc.path",
			content: "[Unit]\n" +
				"Description=tf-acc path\n" +
				"\n" +
				"[Path]\n" +
				"PathExists=/tmp\n" +
				"\n" +
				"[Install]\n" +
				"WantedBy=multi-user.target\n",
			enable:     true,
			wantLoad:   "loaded",
			wantEnable: "enabled",
		},
		{
			name: "tf-acc.socket",
			content: "[Unit]\n" +
				"Description=tf-acc socket\n" +
				"\n" +
				"[Socket]\n" +
				"ListenStream=/run/tf-acc.sock\n" +
				"\n" +
				"[Install]\n" +
				"WantedBy=sockets.target\n",
			enable:     true,
			wantLoad:   "loaded",
			wantEnable: "enabled",
		},
		{
			// Unit name must match What= (/swapfile → swapfile.swap).
			name: "swapfile.swap",
			content: "[Unit]\n" +
				"Description=tf-acc swap\n" +
				"\n" +
				"[Swap]\n" +
				"What=/swapfile\n" +
				"\n" +
				"[Install]\n" +
				"WantedBy=swap.target\n",
			enable:     true,
			wantLoad:   "loaded",
			wantEnable: "enabled",
		},
		{
			name: "tf-acc.slice",
			content: "[Unit]\n" +
				"Description=tf-acc slice\n" +
				"\n" +
				"[Slice]\n" +
				"MemoryMax=64M\n",
			enable:     false,
			wantLoad:   "loaded",
			wantEnable: "static",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var en *bool
			if tc.enable {
				en = accBool(true)
			}
			if err := c.PutUnit(ctx, tc.name, tc.content, en, nil); err != nil {
				t.Fatalf("PutUnit %s: %v", tc.name, err)
			}
			t.Cleanup(func() {
				if err := c.DeleteUnit(ctx, tc.name); err != nil {
					t.Logf("cleanup DeleteUnit %s: %v", tc.name, err)
				}
			})

			st, err := c.UnitStatus(ctx, tc.name)
			if err != nil {
				t.Fatalf("UnitStatus %s: %v", tc.name, err)
			}
			if tc.wantLoad != "" && st.LoadState != tc.wantLoad {
				t.Fatalf("expected LoadState=%s for %s, got %+v", tc.wantLoad, tc.name, st)
			}
			if tc.wantEnable != "" && st.UnitFileState != tc.wantEnable {
				t.Fatalf("expected UnitFileState=%s for %s, got %+v", tc.wantEnable, tc.name, st)
			}
		})
	}
}
