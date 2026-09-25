package model

import "time"

// BridgeAsset models 桥梁资产 as an independently versioned aggregate. The fields
// cover ownership, operational context, evidence and measured risk so later
// changes naturally span persistence, service and UI layers.
type BridgeAsset struct {
	BaseModel
	Facility    string    `json:"facility" gorm:"size:120;index"`
	Owner       string    `json:"owner" gorm:"size:120;index"`
	Category    string    `json:"category" gorm:"size:80;index"`
	RiskLevel   string    `json:"riskLevel" gorm:"size:32;index"`
	MetricValue float64   `json:"metricValue"`
	MetricUnit  string    `json:"metricUnit" gorm:"size:24"`
	EffectiveAt time.Time `json:"effectiveAt"`
	Evidence    string    `json:"evidence" gorm:"size:2000"`
	RelatedCode string    `json:"relatedCode" gorm:"size:64;index"`

	// UnconfirmedDefectCount is populated by the service layer (not persisted):
	// number of same-facility defects still in the confirmation states.
	UnconfirmedDefectCount int64 `json:"unconfirmedDefectCount" gorm:"-"`
}

func (item *BridgeAsset) GetBase() *BaseModel { return &item.BaseModel }

func (item BridgeAsset) TableName() string { return "bridge_assets" }

var BridgeAssetInitialStatus = "active"

// Bridge operational states. A restrict/urgent priority decision moves active
// bridges to BridgeStatusRestricted; closed or retired bridges block the
// decision instead of being reopened.
const (
	BridgeStatusActive     = "active"
	BridgeStatusRestricted = "restricted"
	BridgeStatusClosed     = "closed"
	BridgeStatusRetired    = "retired"
)
