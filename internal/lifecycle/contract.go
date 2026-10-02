package lifecycle

// The contract's document types (SPEC-019 SD-1, DESIGN-010 §9). A feature's
// contract is its spec and its dev-plan; a bug's is its report and its
// dev-plan, because the report serves as its spec. Code that asks "is this the
// spec half of a contract?" asks IsSpecType, so a bug travels the same path.

const (
	DocTypeSpec      = "spec"
	DocTypeDevPlan   = "dev_plan"
	DocTypeBugReport = "bug_report"
	DocTypeFindings  = "findings"
)

// IsSpecType reports whether a document type is the first half of a
// contract: a spec, or a bug's report.
func IsSpecType(docType string) bool {
	return docType == DocTypeSpec || docType == DocTypeBugReport
}

// IsContractType reports whether a document type is either half of a
// contract.
func IsContractType(docType string) bool {
	return IsSpecType(docType) || docType == DocTypeDevPlan
}
