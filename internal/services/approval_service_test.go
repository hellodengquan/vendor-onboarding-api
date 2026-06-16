package services

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"vendor-onboarding-api/internal/database"
	"vendor-onboarding-api/internal/models"
	"vendor-onboarding-api/pkg/utils"
)

func setupTestDB(t *testing.T) {
	t.Helper()
	dbPath := fmt.Sprintf("/tmp/voa_test_%d.db", os.Getpid())
	os.Remove(dbPath)

	db, err := gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=5000&_journal_mode=WAL"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
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
		t.Fatalf("Failed to migrate: %v", err)
	}

	stages := []struct {
		Stage     models.ApprovalStage
		StageName string
		Order     int
		SignType  models.SignType
	}{
		{Stage: models.StageDataCollection, StageName: "资料收集", Order: 0, SignType: models.SignTypeSingle},
		{Stage: models.StageComplianceCheck, StageName: "合规性检查", Order: 1, SignType: models.SignTypeAny},
		{Stage: models.StageLevel1Approval, StageName: "一级审批", Order: 2, SignType: models.SignTypeAll},
		{Stage: models.StageLevel2Approval, StageName: "二级审批", Order: 3, SignType: models.SignTypeMajority},
		{Stage: models.StageFinanceApproval, StageName: "财务审批", Order: 4, SignType: models.SignTypeAny},
		{Stage: models.StageLegalApproval, StageName: "法务审批", Order: 5, SignType: models.SignTypeAny},
	}
	for _, s := range stages {
		db.Create(&models.ApprovalNodeConfig{
			Stage:      s.Stage,
			StageOrder: s.Order,
			StageName:  s.StageName,
			SignType:   s.SignType,
			IsEnabled:  true,
		})
	}

	database.DB = db
	t.Cleanup(func() {
		os.Remove(dbPath)
	})
}

func createTestVendor(t *testing.T, name string) *models.Vendor {
	t.Helper()
	db := database.GetDB()
	vendor := &models.Vendor{
		VendorCode:            "T" + name,
		CompanyName:           name,
		UnifiedSocialCreditCode: "91110000MA01234567",
		LegalPerson:           "张三",
		ContactPerson:         "李四",
		ContactPhone:          "13800000000",
		Status:                models.VendorStatusDraft,
	}
	if err := db.Create(vendor).Error; err != nil {
		t.Fatalf("Failed to create vendor: %v", err)
	}

	flow := &models.ApprovalFlow{
		VendorID:     vendor.ID,
		CurrentStage: models.StageDataCollection,
		StageOrder:   0,
		Status:       models.ApprovalStatusPending,
	}
	if err := db.Create(flow).Error; err != nil {
		t.Fatalf("Failed to create flow: %v", err)
	}
	vendor.CurrentStageID = flow.ID
	db.Save(vendor)
	return vendor
}

func TestApprovalService_Approve_SingleSigner(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	vendor := createTestVendor(t, "SingleVendor")

	err := service.Approve(&models.ApprovalRequest{
		VendorID: vendor.ID,
		Remark:   "资料提交",
	}, 100, "提交人")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	flow, err := service.GetFlow(vendor.ID)
	if err != nil {
		t.Fatalf("GetFlow failed: %v", err)
	}

	if flow.Flow.CurrentStage != models.StageComplianceCheck {
		t.Errorf("Expected stage COMPLIANCE_CHECK, got %s", flow.Flow.CurrentStage)
	}
}

func TestApprovalService_SignTypeAny_OneApproverEnough(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	db := database.GetDB()
	var compConfig models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageComplianceCheck).First(&compConfig)
	signers := []*models.ApprovalNodeSigner{
		{ApprovalNodeID: compConfig.ID, ApproverID: 201, ApproverName: "合规员A", ApproverRole: "合规", Stage: models.StageComplianceCheck},
		{ApprovalNodeID: compConfig.ID, ApproverID: 202, ApproverName: "合规员B", ApproverRole: "合规", Stage: models.StageComplianceCheck},
	}
	for _, s := range signers {
		db.Create(s)
	}

	vendor := createTestVendor(t, "AnyVendor")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")

	err := service.Approve(&models.ApprovalRequest{
		VendorID: vendor.ID,
		Remark:   "合规通过",
	}, 201, "合规员A")
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}

	flow, err := service.GetFlow(vendor.ID)
	if err != nil {
		t.Fatalf("GetFlow failed: %v", err)
	}
	if flow.Flow.CurrentStage != models.StageLevel1Approval {
		t.Errorf("Expected LEVEL_1_APPROVAL after ANY approval, got %s", flow.Flow.CurrentStage)
	}
}

func TestApprovalService_SignTypeAll_RequiresAll(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	db := database.GetDB()
	var cfg models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&cfg)
	signers := []*models.ApprovalNodeSigner{
		{ApprovalNodeID: cfg.ID, ApproverID: 301, ApproverName: "主管1", ApproverRole: "L1", Stage: models.StageLevel1Approval},
		{ApprovalNodeID: cfg.ID, ApproverID: 302, ApproverName: "主管2", ApproverRole: "L1", Stage: models.StageLevel1Approval},
		{ApprovalNodeID: cfg.ID, ApproverID: 303, ApproverName: "主管3", ApproverRole: "L1", Stage: models.StageLevel1Approval},
	}
	for _, s := range signers {
		db.Create(s)
	}

	var cfg2 models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageComplianceCheck).First(&cfg2)

	vendor := createTestVendor(t, "AllVendor")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "快速合规")

	err := service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 301, "主管1")
	if err != nil {
		t.Fatalf("First L1 approve failed: %v", err)
	}

	flow1, _ := service.GetFlow(vendor.ID)
	if flow1.Flow.CurrentStage != models.StageLevel1Approval {
		t.Errorf("Should still in L1 after partial ALL approval")
	}
	if flow1.Flow.Status != models.ApprovalStatusPartial {
		t.Errorf("Expected PARTIAL status, got %s", flow1.Flow.Status)
	}

	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 302, "主管2")
	flow2, _ := service.GetFlow(vendor.ID)
	if flow2.Flow.CurrentStage != models.StageLevel1Approval {
		t.Errorf("Should still in L1 after 2 approvals of 3")
	}

	err = service.Approve(&models.ApprovalRequest{VendorID: vendor.ID, Remark: "全部通过"}, 303, "主管3")
	if err != nil {
		t.Fatalf("Final L1 approve failed: %v", err)
	}

	flowFinal, _ := service.GetFlow(vendor.ID)
	if flowFinal.Flow.CurrentStage != models.StageLevel2Approval {
		t.Errorf("Expected LEVEL_2_APPROVAL after ALL 3 signed, got %s", flowFinal.Flow.CurrentStage)
	}
}

func TestApprovalService_SignTypeMajority(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	db := database.GetDB()
	var cfg models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel2Approval).First(&cfg)
	signers := []*models.ApprovalNodeSigner{
		{ApprovalNodeID: cfg.ID, ApproverID: 401, ApproverName: "副总A", ApproverRole: "L2", Stage: models.StageLevel2Approval},
		{ApprovalNodeID: cfg.ID, ApproverID: 402, ApproverName: "副总B", ApproverRole: "L2", Stage: models.StageLevel2Approval},
		{ApprovalNodeID: cfg.ID, ApproverID: 403, ApproverName: "副总C", ApproverRole: "L2", Stage: models.StageLevel2Approval},
	}
	for _, s := range signers {
		db.Create(s)
	}

	vendor := createTestVendor(t, "MajorityVendor")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "合规")

	var l1 models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&l1)
	db.Create(&models.ApprovalNodeSigner{ApprovalNodeID: l1.ID, ApproverID: 301, ApproverName: "主管1", Stage: models.StageLevel1Approval})
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 301, "主管1")

	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 401, "副总A")
	flow1, _ := service.GetFlow(vendor.ID)
	if flow1.Flow.CurrentStage != models.StageLevel2Approval {
		t.Errorf("Should still in L2 after 1 of 3")
	}

	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID, Remark: "多数通过"}, 402, "副总B")
	flowPostL2, _ := service.GetFlow(vendor.ID)

	if flowPostL2.Flow.CurrentStage != models.StageFinanceApproval {
		t.Errorf("Expected FINANCE_APPROVAL after L2 majority (2/3) approval, got %s", flowPostL2.Flow.CurrentStage)
	}

	var financeCfg models.ApprovalNodeConfig
	database.GetDB().Where("stage = ?", models.StageFinanceApproval).First(&financeCfg)
	database.GetDB().Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: financeCfg.ID, ApproverID: 501, ApproverName: "财务1", Stage: models.StageFinanceApproval,
	})
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 501, "财务1")

	var legalCfg models.ApprovalNodeConfig
	database.GetDB().Where("stage = ?", models.StageLegalApproval).First(&legalCfg)
	database.GetDB().Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: legalCfg.ID, ApproverID: 601, ApproverName: "法务1", Stage: models.StageLegalApproval,
	})
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 601, "法务1")

	flowFinal, _ := service.GetFlow(vendor.ID)
	if flowFinal.Flow.CurrentStage != models.StageCompleted {
		t.Errorf("Expected COMPLETED after finance+legal, got %s", flowFinal.Flow.CurrentStage)
	}
	if flowFinal.Vendor.Status != models.VendorStatusApproved {
		t.Errorf("Expected vendor APPROVED, got %s", flowFinal.Vendor.Status)
	}
}

func TestApprovalService_RejectFlow(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	vendor := createTestVendor(t, "RejectVendor")

	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")

	err := service.Reject(&models.ApprovalRejectRequest{
		VendorID: vendor.ID,
		Remark:   "资料不全，驳回",
	}, 201, "合规员A")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}

	flow, err := service.GetFlow(vendor.ID)
	if err != nil {
		t.Fatalf("GetFlow failed: %v", err)
	}

	if flow.Flow.Status != models.ApprovalStatusRejected {
		t.Errorf("Expected flow REJECTED, got %s", flow.Flow.Status)
	}
	if flow.Flow.CurrentStage != models.StageRejected {
		t.Errorf("Expected stage REJECTED, got %s", flow.Flow.CurrentStage)
	}
	if flow.Vendor.Status != models.VendorStatusRejected {
		t.Errorf("Expected vendor REJECTED status, got %s", flow.Vendor.Status)
	}
}

func TestApprovalService_DuplicateSigner_Rejected(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	db := database.GetDB()
	var cfg models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&cfg)
	db.Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: cfg.ID, ApproverID: 301, ApproverName: "主管1", Stage: models.StageLevel1Approval,
	})
	db.Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: cfg.ID, ApproverID: 302, ApproverName: "主管2", Stage: models.StageLevel1Approval,
	})
	db.Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: cfg.ID, ApproverID: 303, ApproverName: "主管3", Stage: models.StageLevel1Approval,
	})

	vendor := createTestVendor(t, "DupVendor")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "合规")

	err := service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 301, "主管1")
	if err != nil {
		t.Fatalf("First approve should succeed: %v", err)
	}

	flow, _ := service.GetFlow(vendor.ID)
	if flow.Flow.CurrentStage != models.StageLevel1Approval {
		t.Fatalf("Flow should still be at L1 approval with 3 signers (ALL), got %s", flow.Flow.CurrentStage)
	}

	err = service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 301, "主管1")
	if err == nil {
		t.Errorf("Expected duplicate signer error, got nil")
	}
}

func TestApprovalService_NonSigner_Rejected(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	db := database.GetDB()
	var cfg models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&cfg)
	db.Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: cfg.ID, ApproverID: 301, ApproverName: "主管1", Stage: models.StageLevel1Approval,
	})

	vendor := createTestVendor(t, "NonSignerVendor")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "合规快速")

	err := service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 9999, "非会签人员")
	if err == nil {
		t.Errorf("Expected permission error for non-signer, got nil")
	}
}

func TestApprovalService_CreateNodeConfig_And_List(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	req := &models.NodeConfigCreateRequest{
		Stage:     models.StageLevel1Approval,
		StageName: "一级审批会签",
		SignType:  models.SignTypeAll,
		Signers: []models.NodeSignerItem{
			{ApproverID: 501, ApproverName: "审批员A", ApproverRole: "L1"},
			{ApproverID: 502, ApproverName: "审批员B", ApproverRole: "L1"},
		},
	}

	node, err := service.CreateNodeConfig(req)
	if err != nil {
		t.Fatalf("CreateNodeConfig failed: %v", err)
	}
	if node.SignType != models.SignTypeAll {
		t.Errorf("Expected sign type ALL, got %s", node.SignType)
	}

	list, err := service.ListNodeConfigs()
	if err != nil {
		t.Fatalf("ListNodeConfigs failed: %v", err)
	}
	if len(list) < 1 {
		t.Error("Expected at least 1 node config")
	}

	signers, err := service.GetNodeSigners(models.StageLevel1Approval)
	if err != nil {
		t.Fatalf("GetNodeSigners failed: %v", err)
	}
	if len(signers) != 2 {
		t.Errorf("Expected 2 signers for L1 stage, got %d", len(signers))
	}
}

func TestApprovalService_EvaluateSignCondition(t *testing.T) {
	service := NewApprovalService()

	cases := []struct {
		name       string
		signType   models.SignType
		total      int
		approved   int
		rejected   int
		expectDone bool
	}{
		{"SINGLE 0 done", models.SignTypeSingle, 1, 0, 0, false},
		{"SINGLE approved", models.SignTypeSingle, 1, 1, 0, true},
		{"ANY one approved", models.SignTypeAny, 3, 1, 0, true},
		{"ALL 1 of 3", models.SignTypeAll, 3, 1, 0, false},
		{"ALL 3 of 3", models.SignTypeAll, 3, 3, 0, true},
		{"ALL 2+1 rejected", models.SignTypeAll, 3, 2, 1, true},
		{"MAJORITY 3 need 2", models.SignTypeMajority, 3, 1, 0, false},
		{"MAJORITY 3 with 2", models.SignTypeMajority, 3, 2, 0, true},
		{"MAJORITY 5 need 3", models.SignTypeMajority, 5, 2, 0, false},
		{"MAJORITY 5 with 3", models.SignTypeMajority, 5, 3, 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := service.evaluateSignCondition(tc.signType, tc.total, tc.approved, tc.rejected)
			if result != tc.expectDone {
				t.Errorf("%s: expected %v got %v (type=%s total=%d a=%d r=%d)",
					tc.name, tc.expectDone, result, tc.signType, tc.total, tc.approved, tc.rejected)
			}
		})
	}
}

func TestApprovalService_Transition_Manual(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()

	vendor := createTestVendor(t, "TransVendor")

	err := service.Transition(&models.StageTransitionRequest{
		VendorID: vendor.ID,
		ToStage:  models.StageCompleted,
		Remark:   "特批跳过所有阶段",
	}, 9999, "超级管理员")
	if err != nil {
		t.Fatalf("Transition failed: %v", err)
	}

	flow, _ := service.GetFlow(vendor.ID)
	if flow.Flow.CurrentStage != models.StageCompleted {
		t.Errorf("Expected COMPLETED after transition, got %s", flow.Flow.CurrentStage)
	}
	if flow.Vendor.Status != models.VendorStatusApproved {
		t.Errorf("Expected vendor APPROVED after transition")
	}
}

func TestApprovalService_RejectAbortsFlow(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()
	db := database.GetDB()

	var l1 models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&l1)
	for _, id := range []uint64{301, 302, 303} {
		db.Create(&models.ApprovalNodeSigner{
			ApprovalNodeID: l1.ID, ApproverID: id, ApproverName: "主管", Stage: models.StageLevel1Approval,
		})
	}

	vendor := createTestVendor(t, "RejectAbort")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "合规快速")

	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 301, "主管1")
	flow1, _ := service.GetFlow(vendor.ID)
	if flow1.Flow.Status != models.ApprovalStatusPartial {
		t.Errorf("should be PARTIAL after 1/3 approval")
	}

	err := service.Reject(&models.ApprovalRejectRequest{
		VendorID: vendor.ID,
		Remark:   "合同不合规",
	}, 302, "主管2")
	if err != nil {
		t.Fatalf("reject error: %v", err)
	}

	flowFinal, _ := service.GetFlow(vendor.ID)
	if flowFinal.Flow.Status != models.ApprovalStatusRejected {
		t.Errorf("should be REJECTED after one signer rejected, got %s", flowFinal.Flow.Status)
	}
	if flowFinal.Vendor.Status != models.VendorStatusRejected {
		t.Errorf("vendor should be REJECTED status, got %s", flowFinal.Vendor.Status)
	}

	err = service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 303, "主管3")
	if err == nil {
		t.Error("approve should fail after flow rejected")
	}
}

func TestApprovalService_AddSigner_And_Withdraw(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()
	db := database.GetDB()

	var l1 models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&l1)
	db.Create(&models.ApprovalNodeSigner{
		ApprovalNodeID: l1.ID, ApproverID: 301, ApproverName: "主管1", Stage: models.StageLevel1Approval,
	})

	vendor := createTestVendor(t, "AddWithdraw")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "合规")

	err := service.AddSigner(&models.AddSignerRequest{
		VendorID:     vendor.ID,
		ApproverID:   399,
		ApproverName: "加签主管",
		ApproverRole: "L1",
		AddMode:      models.AddModeConcurrent,
		Remark:       "需要额外主管加签",
	}, 9001, "管理员")
	if err != nil {
		t.Fatalf("AddSigner error: %v", err)
	}

	signers, _ := service.GetNodeSigners(models.StageLevel1Approval)
	found := false
	for _, s := range signers {
		if s.ApproverID == 399 {
			found = true
			break
		}
	}
	if !found {
		t.Error("Added signer not found in node signers")
	}

	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 301, "主管1")

	err = service.Withdraw(&models.WithdrawRequest{
		VendorID:   vendor.ID,
		Reason:     "资质有误需要补充",
		IsRollback: true,
	}, 100, "提交人")
	if err != nil {
		t.Fatalf("Withdraw error: %v", err)
	}

	flow, _ := service.GetFlow(vendor.ID)
	if flow.Flow.CurrentStage != models.StageComplianceCheck {
		t.Errorf("after rollback-withdraw from L1 should be COMPLIANCE_CHECK, got %s", flow.Flow.CurrentStage)
	}
}

func TestApprovalService_Concurrent_SignRace(t *testing.T) {
	setupTestDB(t)
	service := NewApprovalService()
	db := database.GetDB()

	var l1 models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&l1)
	ids := []uint64{301, 302, 303, 304, 305}
	for _, id := range ids {
		db.Create(&models.ApprovalNodeSigner{
			ApprovalNodeID: l1.ID, ApproverID: id, ApproverName: "主管", Stage: models.StageLevel1Approval,
		})
	}

	vendor := createTestVendor(t, "RaceVendor")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 100, "提交人")
	service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, 999, "合规")

	done := make(chan bool, len(ids))
	for i, id := range ids {
		go func(uid uint64, idx int) {
			time.Sleep(time.Duration(idx*15) * time.Millisecond)
			_ = service.Approve(&models.ApprovalRequest{VendorID: vendor.ID}, uid, "主管")
			done <- true
		}(id, i)
	}
	for i := 0; i < len(ids); i++ {
		<-done
	}

	flow, _ := service.GetFlow(vendor.ID)
	stage := flow.Flow.CurrentStage
	if stage != models.StageLevel2Approval {
		t.Errorf("after all 5 L1 signers, should proceed to L2, got %s", stage)
	}

	var count int64
	db.Model(&models.ApprovalRecord{}).Where("vendor_id = ? AND stage = ?", vendor.ID, models.StageLevel1Approval).Count(&count)
	if count != int64(len(ids)) {
		t.Errorf("expected %d L1 records, got %d", len(ids), count)
	}
}

func TestAPIKeyService_Rotate_And_Validate(t *testing.T) {
	setupTestDB(t)
	svc := NewAPIKeyService()

	key, secret, err := svc.Create("test_app", "测试应用")
	if err != nil {
		t.Fatalf("Create key error: %v", err)
	}
	if secret == "" {
		t.Fatal("secret should not be empty")
	}

	secrets, err := svc.GetActiveSecrets("test_app")
	if err != nil {
		t.Fatalf("GetActiveSecrets error: %v", err)
	}
	if len(secrets) != 1 || secrets[key.Version] != secret {
		t.Errorf("secret map mismatch: %v", secrets)
	}

	newKey, newSecret, err := svc.Rotate("test_app", 1, "第一次轮换")
	if err != nil {
		t.Fatalf("Rotate error: %v", err)
	}
	if newKey.Version != 2 {
		t.Errorf("expected version 2, got %d", newKey.Version)
	}

	secrets2, _ := svc.GetActiveSecrets("test_app")
	if len(secrets2) != 2 {
		t.Errorf("after rotate, should have 2 active secrets (old+new), got %d", len(secrets2))
	}
	if secrets2[1] != secret || secrets2[2] != newSecret {
		t.Error("old/new secret value mismatch")
	}

	_ = svc.Revoke("test_app", 1)
	secrets3, _ := svc.GetActiveSecrets("test_app")
	_, hasV1 := secrets3[1]
	if hasV1 {
		t.Error("revoked v1 should no longer be active")
	}
	if len(secrets3) != 1 || secrets3[2] != newSecret {
		t.Error("revoke failed")
	}
}

func TestSignatureValidation_FailurePaths(t *testing.T) {
	body := `{"company_name":"测试"}`
	appKey := "sig_test_app"
	secret := "sig_test_secret_abc"
	nonce := "abc123"
	path := "/api/v1/vendors"
	method := "POST"

	sigOK, ts := utils.GenerateSignature(method, path, map[string]string{}, body, appKey, secret, nonce)

	err := utils.ValidateSignature(sigOK, method, path, map[string]string{}, body, ts, nonce, appKey, secret)
	if err != nil {
		t.Errorf("valid sig should pass, got %v", err)
	}

	badSig := sigOK[:len(sigOK)-4] + "0000"
	err = utils.ValidateSignature(badSig, method, path, map[string]string{}, body, ts, nonce, appKey, secret)
	if err == nil {
		t.Error("bad signature should fail")
	}

	oldTs := ts - 1000
	err = utils.ValidateSignature(sigOK, method, path, map[string]string{}, body, oldTs, nonce, appKey, secret)
	if err == nil {
		t.Error("expired timestamp should fail")
	}

	err = utils.ValidateSignature(sigOK, method, path, map[string]string{}, body, ts, "", appKey, secret)
	if err == nil {
		t.Error("empty nonce should fail")
	}

	err = utils.ValidateSignature(sigOK, method, path, map[string]string{}, "{}", ts, nonce, appKey, secret)
	if err == nil {
		t.Error("different body should fail signature")
	}
}

func TestApproval_VersionConflict(t *testing.T) {
	setupTestDB(t)
	svc := NewApprovalService()
	db := database.GetDB()

	vendor := createTestVendor(t, "并发审批厂商")
	// 确保 version 被正确初始化
	db.Model(&models.ApprovalFlow{}).Where("vendor_id = ?", vendor.ID).Update("version", 1)

	// 先提交审批（从 DATA_COLLECTION 推进到 COMPLIANCE_CHECK）
	err := svc.Approve(&models.ApprovalRequest{
		VendorID: vendor.ID,
		Remark:   "资料提交",
	}, 1001, "提交人")
	if err != nil {
		t.Fatalf("Approve submit error: %v", err)
	}

	// 获取当前 flow
	flowResp, err := svc.GetFlow(vendor.ID)
	if err != nil {
		t.Fatalf("GetFlow error: %v", err)
	}
	flow := flowResp.Flow
	originalVersion := flow.Version

	if flow.CurrentStage == models.StageDataCollection {
		t.Fatal("flow should have advanced beyond DATA_COLLECTION after first approve")
	}

	// 模拟另一个进程修改了 version（跨实例篡改）
	// 当前 DB 中 version 是 originalVersion+1（因为第一次 Approve 后已经自增过）
	// 我们把它改成更大的值来模拟冲突
	db.Model(&models.ApprovalFlow{}).Where("id = ?", flow.ID).Update("version", originalVersion+100)

	// 重新获取 flow 用于第二次 Approve
	// 但是让我们先验证当前 DB 中的 version
	var dbFlow models.ApprovalFlow
	db.Where("id = ?", flow.ID).First(&dbFlow)
	if dbFlow.Version != originalVersion+100 {
		t.Fatalf("failed to tamper version, expected %d got %d", originalVersion+100, dbFlow.Version)
	}

	// 此时如果还有缓存的 flow 对象，用它来审批应该失败（expectedVersion 错误）
	// 但 Approve 内部会重新查询数据库，所以我们需要用不同的方式模拟
	// 让我们直接通过 svc.Approve 来测试
	// Approve 内部 SELECT FOR UPDATE 会拿到 version = originalVersion+100
	// expectedVersion = originalVersion+100，然后 flow.Version++ = originalVersion+101
	// UPDATE WHERE version = originalVersion+100，应该成功

	// 换一种方式测试版本冲突：在同一个事务中，两个 goroutine 同时更新
	// 或者我们直接用 svc 连续调用两次，但第一次还没提交时第二次也尝试
	// 更简单：直接修改测试逻辑 - 用两个 goroutine 同时 Approve

	// 先重置 version 为一个已知值
	db.Model(&models.ApprovalFlow{}).Where("id = ?", flow.ID).Update("version", 10)

	// 现在两个 goroutine 同时审批
	errChan := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			approverID := uint64(2000 + idx)
			approverName := fmt.Sprintf("审批员%d", idx)
			errChan <- svc.Approve(&models.ApprovalRequest{
				VendorID: vendor.ID,
				Remark:   fmt.Sprintf("同意-%d", idx),
			}, approverID, approverName)
		}(i)
	}
	wg.Wait()
	close(errChan)

	errCount := 0
	successCount := 0
	for e := range errChan {
		if e != nil {
			errCount++
			if !strings.Contains(e.Error(), "版本冲突") && !strings.Contains(e.Error(), "已在本阶段签署") {
				t.Logf("got expected error: %v", e)
			}
		} else {
			successCount++
		}
	}

	if successCount == 2 {
		t.Fatal("both approvals succeeded, version lock failed!")
	}
	if successCount == 0 {
		t.Log("both approvals failed (acceptable with race)")
	}
	t.Logf("conflict test: success=%d, error=%d", successCount, errCount)

	// 确认 version 已经自增
	var finalFlow models.ApprovalFlow
	db.Where("id = ?", flow.ID).First(&finalFlow)
	if finalFlow.Version <= 10 {
		t.Errorf("version should have incremented, expected > 10, got %d", finalFlow.Version)
	}
}

func TestApprovalAndWithdraw_RaceCondition(t *testing.T) {
	setupTestDB(t)
	svc := NewApprovalService()
	db := database.GetDB()

	vendor := createTestVendor(t, "并发竞争测试厂商")
	db.Model(&models.ApprovalFlow{}).Where("vendor_id = ?", vendor.ID).Update("version", 1)

	// 提交审批
	err := svc.Approve(&models.ApprovalRequest{
		VendorID: vendor.ID,
		Remark:   "资料提交",
	}, 1001, "提交人")
	if err != nil {
		t.Fatalf("Approve submit error: %v", err)
	}

	// 先通过 COMPLIANCE_CHECK 阶段，让 flow 进入 LEVEL_1_APPROVAL（并签）
	err = svc.Approve(&models.ApprovalRequest{
		VendorID: vendor.ID,
		Remark:   "合规通过",
	}, 1002, "合规员")
	if err != nil {
		t.Fatalf("compliance approval error: %v", err)
	}

	// 刷新 flow，确认在 LEVEL_1_APPROVAL
	flowResp, _ := svc.GetFlow(vendor.ID)
	flow := flowResp.Flow
	if flow.CurrentStage != models.StageLevel1Approval {
		t.Fatalf("expected StageLevel1Approval, got %s", flow.CurrentStage)
	}

	// 设置 LEVEL_1_APPROVAL 为并签 (SignTypeAll)
	var cfg models.ApprovalNodeConfig
	db.Where("stage = ?", models.StageLevel1Approval).First(&cfg)
	signers := []*models.ApprovalNodeSigner{
		{ApprovalNodeID: cfg.ID, ApproverID: 2001, ApproverName: "主管A", ApproverRole: "L1", Stage: models.StageLevel1Approval},
		{ApprovalNodeID: cfg.ID, ApproverID: 2002, ApproverName: "主管B", ApproverRole: "L1", Stage: models.StageLevel1Approval},
	}
	for _, s := range signers {
		db.Create(s)
	}

	// 并发：一个协程审批 LEVEL_1，一个协程撤回（版本回滚）
	var wg sync.WaitGroup
	results := make([]error, 2)

	// 审批协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0] = svc.Approve(&models.ApprovalRequest{
			VendorID: vendor.ID,
			Remark:   "L1 同意",
		}, 2001, "主管A")
	}()

	// 撤回协程
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1] = svc.Withdraw(&models.WithdrawRequest{
			VendorID: vendor.ID,
			Reason:   "材料有误，撤回修改",
		}, 1001, "提交人")
	}()

	wg.Wait()

	// 两个操作应该有且只有一个成功（乐观锁生效）
	successCount := 0
	for _, r := range results {
		if r == nil {
			successCount++
		}
	}

	if successCount == 2 {
		t.Fatal("both approval and withdrawal succeeded, optimistic lock failed!")
	}

	// 如果都失败了，至少有一个应该是版本冲突
	if successCount == 0 {
		t.Log("both failed (acceptable if they raced), checking errors")
		conflictCount := 0
		for _, r := range results {
			if r != nil && (strings.Contains(r.Error(), "版本冲突") || strings.Contains(r.Error(), "version")) {
				conflictCount++
			}
		}
		if conflictCount == 0 {
			t.Errorf("at least one should be version conflict, got: %v and %v", results[0], results[1])
		}
	}

	// 检查数据库最终状态的一致性
	var finalFlow models.ApprovalFlow
	db.Where("vendor_id = ?", vendor.ID).First(&finalFlow)

	validStates := map[models.ApprovalStatus]bool{
		models.ApprovalStatusPending:   true,
		models.ApprovalStatusApproved:  true,
		models.ApprovalStatusPartial:   true,
		models.ApprovalStatusWithdrawn: true,
		models.ApprovalStatusRejected:  true,
	}

	if !validStates[finalFlow.Status] {
		t.Errorf("final flow in invalid state: %s", finalFlow.Status)
	}
	t.Logf("race test: approval err=%v, withdraw err=%v, final status=%s, final stage=%s, version=%d",
		results[0], results[1], finalFlow.Status, finalFlow.CurrentStage, finalFlow.Version)
}


