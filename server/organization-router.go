package main

import (
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

type OrganizationRouter struct {
}

type CreateOrganizationRequest struct {
	Name      string `json:"name" validate:"required"`
	Firstname string `json:"firstname" validate:"required"`
	Lastname  string `json:"lastname" validate:"required"`
	Email     string `json:"email" validate:"required,email"`
	Country   string `json:"country" validate:"required,len=2"`
	Language  string `json:"language" validate:"required,len=2"`
}

type GetOrganizationResponse struct {
	ID string `json:"id"`
	CreateOrganizationRequest
}

type GetDomainResponse struct {
	DomainName  string `json:"domain"`
	Active      bool   `json:"active"`
	VerifyToken string `json:"verifyToken"`
}

type GetManageSubscriptionURLResponse struct {
	URL string `json:"url"`
}

func (router *OrganizationRouter) setupRoutes(s *mux.Router) {
	s.HandleFunc("/domain/{domain}", router.getOrgForDomain).Methods("GET")
	s.HandleFunc("/{id}/subscription/manage", router.getManageSubscriptionURL).Methods("GET")
	s.HandleFunc("/{id}/domain/", router.getDomains).Methods("GET")
	s.HandleFunc("/{id}/domain/{domain}/verify", router.verifyDomain).Methods("POST")
	s.HandleFunc("/{id}/domain/{domain}", router.removeDomain).Methods("DELETE")
	s.HandleFunc("/{id}/domain/{domain}", router.addDomain).Methods("POST")
	s.HandleFunc("/{id}", router.getOne).Methods("GET")
	s.HandleFunc("/{id}", router.update).Methods("PUT")
	s.HandleFunc("/{id}", router.delete).Methods("DELETE")
	s.HandleFunc("/", router.create).Methods("POST")
	s.HandleFunc("/", router.getAll).Methods("GET")
}

func (router *OrganizationRouter) getManageSubscriptionURL(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	org, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, org.ID)) {
		SendForbidden(w)
		return
	}
	api := &FastSpringAPI{}
	accountID, err := router.getCreateFastSpringAccountID(api, org)
	if err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	subscriptionID, _ := GetSettingsRepository().Get(org.ID, SettingFastSpringSubscriptionID.Name)
	var url string
	if subscriptionID == "" {
		url, err = api.StartCheckoutSession(accountID, FastSpringProduct50Users, 1, !GetConfig().Development)
	} else {
		url, err = api.Authenticate(accountID)
	}
	if err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	res := &GetManageSubscriptionURLResponse{
		URL: url,
	}
	SendJSON(w, res)
}

func (router *OrganizationRouter) getCreateFastSpringAccountID(api *FastSpringAPI, org *Organization) (string, error) {
	accountID, err := GetSettingsRepository().Get(org.ID, SettingFastSpringAccountID.Name)
	if (err != nil) || accountID == "" {
		req := &FastSpringCreateAccountRequest{
			Contact: FastSpringContact{
				Company:   org.Name,
				Firstname: org.ContactFirstname,
				Lastname:  org.ContactLastname,
				Email:     org.ContactEmail,
			},
			Country:  org.Country,
			Language: org.Language,
			Lookup: FastSpringLookup{
				CustomID: org.ID,
			},
		}
		accountID, err = api.CreateAccount(req)
		if err != nil {
			return "", err
		}
		GetSettingsRepository().Set(org.ID, SettingFastSpringAccountID.Name, accountID)
	}
	return accountID, nil
}

func (router *OrganizationRouter) getOrgForDomain(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOneByDomain(vars["domain"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	res := &GetOrganizationResponse{}
	res.ID = e.ID
	res.Name = e.Name
	SendJSON(w, res)
}

func (router *OrganizationRouter) getOne(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, e.ID)) {
		SendForbidden(w)
		return
	}
	res := router.copyToRestModel(e)
	SendJSON(w, res)
}

func (router *OrganizationRouter) getAll(w http.ResponseWriter, r *http.Request) {
	user := GetRequestUser(r)
	if !user.SuperAdmin {
		SendForbidden(w)
		return
	}
	list, err := GetOrganizationRepository().GetAll()
	if err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	res := []*GetOrganizationResponse{}
	for _, e := range list {
		m := router.copyToRestModel(e)
		res = append(res, m)
	}
	SendJSON(w, res)
}

func (router *OrganizationRouter) getDomains(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, e.ID)) {
		SendForbidden(w)
		return
	}
	list, err := GetOrganizationRepository().GetDomains(e)
	if err != nil {
		SendInternalServerError(w)
		return
	}
	res := []*GetDomainResponse{}
	for _, domain := range list {
		item := &GetDomainResponse{
			DomainName:  domain.DomainName,
			Active:      domain.Active,
			VerifyToken: domain.VerifyToken,
		}
		res = append(res, item)
	}
	SendJSON(w, res)
}

func (router *OrganizationRouter) addDomain(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, e.ID)) {
		SendForbidden(w)
		return
	}
	// Check if domain exists in this org already
	domain, _ := GetOrganizationRepository().GetDomain(e, vars["domain"])
	if domain != nil {
		SendAleadyExists(w)
		return
	}
	// Check if domain exists in activated state in ANY org already
	someOrg, _ := GetOrganizationRepository().GetOneByDomain(vars["domain"])
	if someOrg != nil {
		SendAleadyExists(w)
		return
	}
	// Add domain
	err = GetOrganizationRepository().AddDomain(e, vars["domain"], false)
	if err != nil {
		log.Println(err)
		SendAleadyExists(w)
		return
	}
	SendCreated(w, vars["domain"])
}

func (router *OrganizationRouter) verifyDomain(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, e.ID)) {
		SendForbidden(w)
		return
	}
	domain, err := GetOrganizationRepository().GetDomain(e, vars["domain"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	if domain.Active {
		return
	}
	// Check if domain exists in activated state in ANY org already
	someOrg, _ := GetOrganizationRepository().GetOneByDomain(vars["domain"])
	if someOrg != nil {
		SendAleadyExists(w)
		return
	}
	if !router.isValidTXTRecord(domain.DomainName, domain.VerifyToken) {
		SendBadRequest(w)
		return
	}
	err = GetOrganizationRepository().ActivateDomain(e, domain.DomainName)
	if err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	SendUpdated(w)
}

func (router *OrganizationRouter) removeDomain(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		log.Println(err)
		SendNotFound(w)
		return
	}
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, e.ID)) {
		SendForbidden(w)
		return
	}
	mailParts := strings.Split(user.Email, "@")
	if len(mailParts) != 2 {
		SendInternalServerError(w)
		return
	}
	domain := strings.ToLower(mailParts[1])
	if strings.ToLower(vars["domain"]) == domain {
		SendBadRequest(w)
		return
	}
	err = GetOrganizationRepository().RemoveDomain(e, vars["domain"])
	if err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	SendUpdated(w)
}

func (router *OrganizationRouter) update(w http.ResponseWriter, r *http.Request) {
	user := GetRequestUser(r)
	if !user.SuperAdmin {
		SendForbidden(w)
		return
	}
	var m CreateOrganizationRequest
	if UnmarshalValidateBody(r, &m) != nil {
		SendBadRequest(w)
		return
	}
	vars := mux.Vars(r)
	e := router.copyFromRestModel(&m)
	e.ID = vars["id"]
	if err := GetOrganizationRepository().Update(e); err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	SendUpdated(w)
}

func (router *OrganizationRouter) delete(w http.ResponseWriter, r *http.Request) {
	user := GetRequestUser(r)
	if !(user.SuperAdmin || CanAdminOrg(user, user.OrganizationID)) {
		SendForbidden(w)
		return
	}
	vars := mux.Vars(r)
	e, err := GetOrganizationRepository().GetOne(vars["id"])
	if err != nil {
		SendNotFound(w)
		return
	}
	if err := GetOrganizationRepository().Delete(e); err != nil {
		SendInternalServerError(w)
		return
	}
	SendUpdated(w)
}

func (router *OrganizationRouter) create(w http.ResponseWriter, r *http.Request) {
	user := GetRequestUser(r)
	if !user.SuperAdmin {
		SendForbidden(w)
		return
	}
	var m CreateOrganizationRequest
	if err := UnmarshalValidateBody(r, &m); err != nil {
		SendBadRequest(w)
		return
	}
	e := router.copyFromRestModel(&m)
	e.SignupDate = time.Now()
	if err := GetOrganizationRepository().Create(e); err != nil {
		log.Println(err)
		SendInternalServerError(w)
		return
	}
	SendCreated(w, e.ID)
}

func (router *OrganizationRouter) isValidTXTRecord(domain, uuid string) bool {
	records, err := net.LookupTXT(domain)
	if err != nil {
		log.Println(err)
		return false
	}
	checkString := "seatsurfing-verification=" + uuid
	for _, record := range records {
		if record == checkString {
			return true
		}
	}
	return false
}

func (router *OrganizationRouter) copyFromRestModel(m *CreateOrganizationRequest) *Organization {
	e := &Organization{}
	e.Name = m.Name
	e.ContactFirstname = m.Firstname
	e.ContactLastname = m.Lastname
	e.ContactEmail = m.Email
	e.Country = m.Country
	e.Language = m.Language
	return e
}

func (router *OrganizationRouter) copyToRestModel(e *Organization) *GetOrganizationResponse {
	m := &GetOrganizationResponse{}
	m.ID = e.ID
	m.Name = e.Name
	m.Firstname = e.ContactFirstname
	m.Lastname = e.ContactLastname
	m.Email = e.ContactEmail
	m.Country = e.Country
	m.Language = e.Language
	return m
}
