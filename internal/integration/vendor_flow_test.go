package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/middleware"
	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/pkg/utils"
)

func initTestDatabase(t *testing.T) {
	t.Helper()
	dbPath := fmt.Sprintf("/tmp/voa_integration_%d.db", os.Getpid())
	os.Remove(dbPath)

	db, err := gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	err = db.AutoMigrate(
		&models.Vendor{},
		&models.Qualification{},
		&models.ApprovalFlow{},
		&models.ApprovalRecord{},
		&models.ApprovalNodeConfig{},
		&models.ApprovalNodeSigner{},
		&models.ParallelGroup{},
		&models.ParallelGroupInstance{},
		&models.AdditionalSignerRecord{},
		&models.WithdrawalRecord{},
		&models.APIKey{},
	)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	stages := []struct {
		Stage     models.ApprovalStage
		StageName string
		Order     int
		SignType  models.SignType
	}{
		{models.StageDataCollection, "资料收集", 0, models.SignTypeSingle},
		{models.StageComplianceCheck, "合规性检查", 1, models.SignTypeAny},
		{models.StageLevel1Approval, "一级审批", 2, models.SignTypeAll},
		{models.StageLevel2Approval, "二级审批", 3, models.SignTypeMajority},
		{models.StageFinanceApproval, "财务审批", 4, models.SignTypeAny},
		{models.StageLegalApproval, "法务审批", 5, models.SignTypeAny},
	}
	for _, s := range stages {
		node := &models.ApprovalNodeConfig{
			Stage:      s.Stage,
			StageOrder: s.Order,
			StageName:  s.StageName,
			SignType:   s.SignType,
			IsEnabled:  true,
		}
		db.Create(node)
		switch s.Stage {
		case models.StageComplianceCheck:
			db.Create(&models.ApprovalNodeSigner{ApprovalNodeID: node.ID, Stage: s.Stage, ApproverID: 201, ApproverName: "合规员A", ApproverRole: "合规"})
		case models.StageLevel1Approval:
			for i, id := range []uint64{301, 302, 303} {
				db.Create(&models.ApprovalNodeSigner{ApprovalNodeID: node.ID, Stage: s.Stage, ApproverID: id, ApproverName: fmt.Sprintf("主管%d", i+1), ApproverRole: "L1"})
			}
		case models.StageLevel2Approval:
			for i, id := range []uint64{401, 402, 403} {
				db.Create(&models.ApprovalNodeSigner{ApprovalNodeID: node.ID, Stage: s.Stage, ApproverID: id, ApproverName: fmt.Sprintf("副总%d", i+1), ApproverRole: "L2"})
			}
		case models.StageFinanceApproval:
			db.Create(&models.ApprovalNodeSigner{ApprovalNodeID: node.ID, Stage: s.Stage, ApproverID: 501, ApproverName: "财务1", ApproverRole: "财务"})
		case models.StageLegalApproval:
			db.Create(&models.ApprovalNodeSigner{ApprovalNodeID: node.ID, Stage: s.Stage, ApproverID: 601, ApproverName: "法务1", ApproverRole: "法务"})
		}
	}

	db.Create(&models.APIKey{
		AppKey:    "integration_test",
		AppSecret: "integration-secret-2024",
		Status:    "ACTIVE",
		Version:   1,
	})

	database.DB = db
	t.Cleanup(func() { os.Remove(dbPath) })
}

func signedRequest(t *testing.T, method, path string, body interface{}, userID uint64, userName string) *http.Request {
	t.Helper()
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}
	appKey := "integration_test"
	appSecret := "integration-secret-2024"
	nonce := "test-nonce-" + path
	nonce = nonce[len(nonce)-8:]
	if len(nonce) < 8 {
		nonce = "abcdefgh"
	}

	sig, ts := utils.GenerateSignature(method, path, map[string]string{}, string(bodyBytes), appKey, appSecret, nonce)

	var req *http.Request
	if bodyBytes != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set(utils.SignatureAppKeyKey, appKey)
	req.Header.Set(utils.SignatureTimestampKey, fmt.Sprintf("%d", ts))
	req.Header.Set(utils.SignatureNonceKey, nonce)
	req.Header.Set(utils.SignatureHeaderKey, sig)
	req.Header.Set("X-User-ID", fmt.Sprintf("%d", userID))
	req.Header.Set("X-User-Name", userName)
	return req
}

func parseResponse(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse response: %v, body=%s", err, rec.Body.String())
	}
	return out
}

func TestIntegration_FullVendorApprovalFlow(t *testing.T) {
	initTestDatabase(t)
	r := middleware.SetupRouter()

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, "POST", "/api/v1/vendors",
		models.VendorCreateRequest{
			CompanyName:             "集成测试供应商有限公司",
			UnifiedSocialCreditCode: "91310000MA1FL00X16",
			LegalPerson:             "张集成",
			ContactPerson:           "李测试",
			ContactPhone:            "13800000001",
			ContactEmail:            "integration@test.com",
		}, 100, "供应商录入员"))
	if rec.Code != http.StatusOK {
		t.Fatalf("create vendor failed, code=%d body=%s", rec.Code, rec.Body.String())
	}
	out := parseResponse(t, rec)
	if code, _ := out["code"].(float64); code != 0 {
		t.Fatalf("create vendor failed response: %v", out)
	}
	data, _ := out["data"].(map[string]interface{})
	vendorID := uint64(data["id"].(float64))
	if vendorID == 0 {
		t.Fatalf("vendor id should not be 0: %v", data)
	}

	t.Logf("Created vendor id=%d", vendorID)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, "POST", fmt.Sprintf("/api/v1/vendors/%d/qualifications", vendorID),
		models.QualificationUploadRequest{
			Type:    models.QualificationTypeBusinessLicense,
			Name:    "营业执照",
			FileURL: "https://example.com/license.pdf",
		}, 100, "供应商录入员"))
	out = parseResponse(t, rec)
	if code, _ := out["code"].(float64); code != 0 {
		t.Fatalf("upload qualification failed: %v", out)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, "POST", fmt.Sprintf("/api/v1/vendors/%d/submit", vendorID),
		nil, 100, "供应商录入员"))
	out = parseResponse(t, rec)
	if code, _ := out["code"].(float64); code != 0 {
		t.Fatalf("submit vendor failed: %v", out)
	}

	steps := []struct {
		name     string
		uid      uint64
		uname    string
		reject   bool
	}{
		{"合规通过", 201, "合规员A", false},
		{"L1-主管1通过", 301, "主管1", false},
		{"L1-主管2通过", 302, "主管2", false},
		{"L1-主管3通过", 303, "主管3", false},
		{"L2-副总A通过", 401, "副总1", false},
		{"L2-副总B通过(多数通过)", 402, "副总2", false},
		{"财务通过", 501, "财务1", false},
		{"法务通过", 601, "法务1", false},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			payload := map[string]interface{}{"vendor_id": vendorID, "remark": step.name}
			if step.reject {
				rec = httptest.NewRecorder()
				r.ServeHTTP(rec, signedRequest(t, "POST", fmt.Sprintf("/api/v1/vendors/%d/approval/reject", vendorID),
					payload, step.uid, step.uname))
			} else {
				rec = httptest.NewRecorder()
				r.ServeHTTP(rec, signedRequest(t, "POST", fmt.Sprintf("/api/v1/vendors/%d/approval/approve", vendorID),
					payload, step.uid, step.uname))
			}
			out = parseResponse(t, rec)
			if code, _ := out["code"].(float64); code != 0 {
				t.Fatalf("[%s] failed: %v", step.name, out)
			}
		})
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, signedRequest(t, "GET", fmt.Sprintf("/api/v1/vendors/%d/approval/flow", vendorID),
		nil, 9000, "管理员"))
	out = parseResponse(t, rec)
	data, _ = out["data"].(map[string]interface{})
	flow, _ := data["flow"].(map[string]interface{})
	finalStage := flow["current_stage"].(string)
	if finalStage != string(models.StageCompleted) {
		t.Errorf("Expected final stage COMPLETED, got %s. flow=%v", finalStage, flow)
	}
	vendor, _ := data["vendor"].(map[string]interface{})
	vendorStatus := vendor["status"].(string)
	if vendorStatus != string(models.VendorStatusApproved) {
		t.Errorf("Expected vendor status APPROVED, got %s", vendorStatus)
	}
}

func TestIntegration_Signature_Failures(t *testing.T) {
	initTestDatabase(t)
	r := middleware.SetupRouter()

	req := httptest.NewRequest("GET", "/api/v1/vendors", nil)
	req.Header.Set(utils.SignatureAppKeyKey, "integration_test")
	req.Header.Set(utils.SignatureTimestampKey, fmt.Sprintf("%d", 1))
	req.Header.Set(utils.SignatureNonceKey, "abc")
	req.Header.Set(utils.SignatureHeaderKey, "badbadbad")

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	out := parseResponse(t, rec)
	if code, _ := out["code"].(float64); code != 401 {
		t.Errorf("Expected 401 for bad signature, got %v: %v", out["code"], out)
	}
}

func TestIntegration_VendorValidator_InvalidInput(t *testing.T) {
	initTestDatabase(t)
	r := middleware.SetupRouter()

	badCases := []struct {
		name string
		req  interface{}
	}{
		{"空公司名", models.VendorCreateRequest{
			UnifiedSocialCreditCode: "91110000MA00000001",
			ContactPerson:           "张三",
			ContactPhone:            "13800000000",
		}},
		{"短信用代码", models.VendorCreateRequest{
			CompanyName:             "测试",
			UnifiedSocialCreditCode: "123",
			ContactPerson:           "张三",
			ContactPhone:            "13800000000",
		}},
		{"坏手机号", models.VendorCreateRequest{
			CompanyName:             "测试",
			UnifiedSocialCreditCode: "91110000MA00000001",
			ContactPerson:           "张三",
			ContactPhone:            "abc",
		}},
	}
	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, signedRequest(t, "POST", "/api/v1/vendors", tc.req, 100, "录入员"))
			out := parseResponse(t, rec)
			code, _ := out["code"].(float64)
			if code != 400 {
				t.Errorf("[%s] expected 400 for bad input, got code=%v msg=%v", tc.name, out["code"], out["message"])
			}
		})
	}
}
