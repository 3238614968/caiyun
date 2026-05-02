package handlers

// ErrorResponse 错误响应
type ErrorResponse struct {
	Message string `json:"message" example:"错误信息"`
	Error   string `json:"error,omitempty" example:"详细错误描述"`
}

// InternalServerErrorResponse 返回统一的 500 错误响应，避免把内部路径、
// 上游响应体或数据库错误直接暴露给客户端。
func InternalServerErrorResponse() ErrorResponse {
	return ErrorResponse{Message: "服务器内部错误，请稍后再试"}
}

// SuccessResponse 成功响应
type SuccessResponse struct {
	Message string `json:"message" example:"操作成功"`
}

// PaginationRequest 分页请求参数
type PaginationRequest struct {
	Page     int `form:"page" json:"page" example:"1"`
	PageSize int `form:"page_size" json:"page_size" example:"10"`
}

// PaginationResponse 分页响应
type PaginationResponse struct {
	Total    int64 `json:"total" example:"100"`
	Page     int   `json:"page" example:"1"`
	PageSize int   `json:"page_size" example:"10"`
}

// ListRequest 列表请求
type ListRequest struct {
	PaginationRequest
	SortBy    string `form:"sort_by" json:"sort_by,omitempty" example:"created_at"`
	SortOrder string `form:"sort_order" json:"sort_order,omitempty" example:"desc"`
}

// Validate 验证分页参数
func (p *PaginationRequest) Validate() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = 10
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}
}

// Offset 计算偏移量
func (p *PaginationRequest) Offset() int {
	return (p.Page - 1) * p.PageSize
}
