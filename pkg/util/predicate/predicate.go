package predicate

import (
	hubv1 "github.com/gardener/potter-controller/api/v1"
	"github.com/gardener/potter-controller/pkg/util"

	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func LandscaperManaged() predicate.Predicate {
	return &landscaperManaged{}
}

type landscaperManaged struct{}

func (l *landscaperManaged) Create(ev event.CreateEvent) bool {
	return util.HasLabel(ev.Meta, hubv1.LabelLandscaperManaged, hubv1.LabelValueLandscaperManaged)
}

func (l *landscaperManaged) Delete(ev event.DeleteEvent) bool {
	return util.HasLabel(ev.Meta, hubv1.LabelLandscaperManaged, hubv1.LabelValueLandscaperManaged)
}

func (l *landscaperManaged) Update(ev event.UpdateEvent) bool {
	return util.HasLabel(ev.MetaNew, hubv1.LabelLandscaperManaged, hubv1.LabelValueLandscaperManaged)
}

func (l *landscaperManaged) Generic(ev event.GenericEvent) bool {
	return util.HasLabel(ev.Meta, hubv1.LabelLandscaperManaged, hubv1.LabelValueLandscaperManaged)
}

func Not(p predicate.Predicate) predicate.Predicate {
	return &not{p}
}

type not struct {
	p predicate.Predicate
}

func (n *not) Create(e event.CreateEvent) bool {
	return !n.p.Create(e)
}

func (n *not) Delete(e event.DeleteEvent) bool {
	return !n.p.Delete(e)
}

func (n *not) Update(e event.UpdateEvent) bool {
	return !n.p.Update(e)
}

func (n *not) Generic(e event.GenericEvent) bool {
	return !n.p.Generic(e)
}
