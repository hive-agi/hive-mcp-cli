package addon

import (
	"os"
	"os/exec"
	"strings"
)

// AddonVersion is the hive-addon coordinate `status` puts on the classpath when
// the project does not already carry one. Pinned rather than RELEASE so the
// answer does not change under the user between two runs.
const AddonVersion = "1.0.0"

// Program is the Clojure `status` runs: discover the addons on this project's
// classpath, order them, and ask the mounter what WOULD happen, with no addon
// constructed and nothing initialized.
//
// The verdict is computed by the mounter, in the JVM, under whatever licence
// gate this project installs. It is deliberately not recomputed here. A gate is
// the vendor's signed answer verified against a public key compiled into the
// gated artifact, so a CLI that decided entitlement for itself would be a
// second licence decision in the fleet with none of the key material, and it
// would be wrong in whichever direction was more convenient to code.
//
// Unconfigured hosts read "no licence gate installed", which is the honest
// answer and the true default: hive-addon's closed gate refuses every
// proprietary spec until a host installs one.
const Program = `
(require '[hive-addon.mount.boundary :as b]
         '[hive-addon.mount.solve :as s]
         '[hive-addon.mount.port :as p]
         '[hive-addon.mount.entitlement :as e]
         '[clojure.string :as str])
(let [{:keys [specs errors]} (b/discover-specs)
      plan (s/solve specs)
      report (b/dry-run plan (p/atom-mount-host) {})
      by-id (into {} (map (juxt :addon/id identity)) (:mounted report))]
  (println (format "%-28s %-12s %s" "ADDON" "TRUST" "WOULD MOUNT"))
  (doseq [spec (:ordered plan)
          :let [id (:addon/id spec)
                r (get by-id id)
                trust (name (:addon/trust-class spec :foss))
                verdict (cond
                          (:success? r) "yes"
                          (:deny/reason r) (str "no: " (name (:deny/reason r)))
                          :else (str "no: " (str/join "; " (:errors r))))]]
    (println (format "%-28s %-12s %s" id trust verdict)))
  (when (seq errors)
    (println)
    (println "Unreadable manifests:")
    (doseq [e errors] (println " " (:url e) (str/join "; " (:errors e)))))
  (println)
  (println (format "%d addon(s) on the classpath, %d would mount. Gate: %s."
                   (count (:ordered plan))
                   (count (filter :success? (:mounted report)))
                   (name (e/gate-id (e/installed-gate)))))
  (when (= :closed (e/gate-id (e/installed-gate)))
    (println "The closed gate is the default: no licence is installed, so every")
    (println "proprietary addon is refused. Install one in your host to change that.")))
`

// Args is the argv `status` runs, with hive-addon layered over the project's own
// deps so discovery still sees the project's addons.
//
// -Sdeps MERGES, so a project that already pins hive-addon keeps its pin: this
// only supplies one when the project has none, which is the case for a plain
// user project that has just added a paid addon.
func Args(addonVersion string) []string {
	if addonVersion == "" {
		addonVersion = AddonVersion
	}
	deps := `{:deps {io.github.hive-agi/hive-addon {:mvn/version "` + addonVersion + `"}}}`
	return []string{"clojure", "-Sdeps", deps, "-M", "-e", Program}
}

// Available reports whether the Clojure CLI is on PATH. `status` is the one
// addon command that needs a JVM, and saying so beats an exec error.
func Available() bool {
	_, err := exec.LookPath("clojure")
	return err == nil
}

// Run executes `status` in dir, streaming the mounter's own output.
func Run(dir string, addonVersion string) error {
	args := Args(addonVersion)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// IsProject reports whether dir looks like a Clojure project, so `status` can
// say what is wrong instead of letting the Clojure CLI fail obscurely.
func IsProject(dir string) bool {
	for _, f := range []string{"deps.edn", "bb.edn", "project.clj"} {
		if _, err := os.Stat(strings.TrimRight(dir, "/") + "/" + f); err == nil {
			return true
		}
	}
	return false
}
