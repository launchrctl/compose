package compose

import (
	"testing"
	"testing/fstest"
)

type fsmy map[string]string

func (f fsmy) MapFS() fstest.MapFS {
	m := make(fstest.MapFS)
	for k, v := range f {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func Test_LockLookup(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name   string
		fs     fsmy
		expCfg bool
		expErr bool
	}

	tts := []testCase{
		{"valid config", fsmy{"compose.lock": validLockYml}, true, false},
		{"empty dir", fsmy{}, false, true},
		{"no config", fsmy{"compose.lock.bkp": "test", "my.config.yaml": "test"}, false, true},
		{"invalid config", fsmy{"compose.lock": invalidLockYml}, false, true},
	}
	for _, tt := range tts {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := lockLookup(tt.fs.MapFS())
			if (err == nil) == tt.expErr {
				t.Errorf("unexpected error on lock parsing")
			}
			if (cfg == nil) == tt.expCfg {
				t.Errorf("exected lock result")
			}
		})
	}

}
