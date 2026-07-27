//go:build !skip_acc

package provider

import "testing"

// TestAccTypedUnits writes+enables one unit of each "typed" kind (timer,
// path, socket) via Client.PutUnit + EnableUnit and asserts each loads and
// enables cleanly. None of them are started: a timer/path/socket unit
// without a matching `tf-acc.service` would fail as soon as its trigger
// condition (elapsed time, PathExists, incoming connection) fires, so this
// test only exercises write/enable/status, matching the brief.
func TestAccTypedUnits(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	cases := []struct {
		name    string
		content string
	}{
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
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.PutUnit(ctx, tc.name, tc.content, accBool(true), nil); err != nil {
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
			if st.LoadState != "loaded" {
				t.Fatalf("expected LoadState=loaded for %s, got %+v", tc.name, st)
			}
			if st.UnitFileState != "enabled" {
				t.Fatalf("expected UnitFileState=enabled for %s, got %+v", tc.name, st)
			}
		})
	}
}
