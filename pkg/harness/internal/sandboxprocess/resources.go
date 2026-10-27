package sandboxprocess

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hyscale-lab/aries/pkg/core"
)

// ReadResources runs outside the measured group. It never changes controllers,
// quotas, admission or counters, and remains usable after confirmed Stop.
func (g *Group) ReadResources(ctx context.Context) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := g.sandbox.Exec(ctx, core.Command{Path: "python3", Args: []string{"-I", "-c", ResourceScript, g.cgroup}, User: "root", Dir: "/", Timeout: 2 * time.Second, OutputLimitBytes: 128 << 10})
	if err != nil || result.ExitCode != 0 {
		return nil, errors.New("cgroup resource read command failed")
	}
	raw := json.RawMessage(result.Stdout)
	var identity struct {
		Group string `json:"cgroup_path"`
		Inode string `json:"cgroup_inode"`
	}
	if len(raw) > 128<<10 || json.Unmarshal(raw, &identity) != nil || identity.Group != g.cgroup || identity.Inode == "" {
		return nil, errors.New("invalid cgroup resource read receipt")
	}
	return raw, nil
}

// Each file keeps its original kernel text and unit. Missing controller files
// and a readable empty io.stat are capabilities, not fabricated zero readings.
const ResourceScript = `import datetime,json,os,stat,sys,time
p=sys.argv[1]
st=os.lstat(p)
if not stat.S_ISDIR(st.st_mode) or st.st_uid!=0: raise RuntimeError("untrusted resource group")
start=time.monotonic_ns()
files={}
for name in ("cpu.stat","memory.current","memory.peak","io.stat"):
    try:
        with open(p+"/"+name,"r",encoding="ascii") as f: raw=f.read(16385)
        if len(raw)>16384: files[name]={"status":"failed","error":"size_limit"}
        else: files[name]={"status":"collected" if raw.strip() else "empty","raw":raw}
    except FileNotFoundError: files[name]={"status":"unsupported","error":"missing_file"}
    except OSError as e: files[name]={"status":"failed","error":"errno_"+str(e.errno)}
    except UnicodeError: files[name]={"status":"failed","error":"invalid_ascii"}
end=time.monotonic_ns()
print(json.dumps({"cgroup_path":p,"cgroup_inode":str(st.st_ino),"guest_monotonic_started_ns":str(start),"guest_monotonic_finished_ns":str(end),"observed_at_utc":datetime.datetime.now(datetime.timezone.utc).isoformat(),"files":files},separators=(",",":")))
`
