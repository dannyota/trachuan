package evaluate

import "danny.vn/trachuan/internal/frameworks/build"

func withFailureCauses(r InvariantCaller) InvariantCaller {
	for i := range r.Failures {
		if r.Failures[i].Cause.GoType == "" {
			r.Failures[i].Cause = build.IdentifyFailure(nil)
		}
	}
	return r
}
