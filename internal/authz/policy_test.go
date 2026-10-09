package authz

import (
	"testing"

	"github.com/oig-police/oig-web/internal/model"
)

func TestCatalogWriteRestriction(t *testing.T) {
	p := New()
	if !p.Can(model.RoleOwner, WriteCatalog) || !p.Can(model.RoleAdmin, WriteCatalog) {
		t.Fatal("Owner and Admin must be able to write catalog rules")
	}
	if p.Can(model.RoleScreener, WriteCatalog) || p.Can(model.RoleGuest, WriteCatalog) {
		t.Fatal("Screener and Guest must not write catalog rules")
	}
}

func TestRoleHierarchy(t *testing.T) {
	p := New()
	owner := model.User{UserID: "owner", Role: model.RoleOwner}
	screener := model.User{UserID: "screener", Role: model.RoleScreener}
	admin := model.User{UserID: "admin", Role: model.RoleAdmin}
	guest := model.User{UserID: "guest", Role: model.RoleGuest}

	if !p.CanChangeRole(owner, admin, model.RoleScreener) {
		t.Fatal("Owner should be able to promote Admin to Screener")
	}
	if p.CanChangeRole(screener, guest, model.RoleScreener) {
		t.Fatal("Screener must not create another Screener")
	}
	if p.CanChangeRole(owner, owner, model.RoleAdmin) {
		t.Fatal("self role changes must be rejected")
	}
	if p.CanChangeRole(owner, admin, model.RoleOwner) {
		t.Fatal("Owner assignment must be out of band")
	}
}
