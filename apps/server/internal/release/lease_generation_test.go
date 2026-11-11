package release

import "testing"

func TestResolver_LeaseGenerationMatchesActivationObservation(t *testing.T) {
	r, s, _ := resolverFixture(t)
	type activation struct {
		release    string
		generation uint64
	}
	var notifications []activation
	r.OnActivate = func(release string, generation uint64) {
		notifications = append(notifications, activation{release, generation})
	}
	for _, name := range []string{"corpus_vn_20990103", "corpus_vn_20990103", "corpus_vn_20990104", "corpus_vn_20990103"} {
		s.active = name
		lease := takeLease(t, r, "")
		generation := lease.Generation
		observed := notifications[len(notifications)-1]
		if lease.Release != observed.release || generation != observed.generation {
			t.Fatalf("lease generation=%d notification generation=%d", generation, observed.generation)
		}
		lease.Done()
	}
	if len(notifications) != 3 {
		t.Fatal("unchanged pointer advanced generation")
	}
	for i, n := range notifications {
		if n.generation != uint64(i+1) {
			t.Fatal("activation sequence did not advance once per notification")
		}
	}
}
