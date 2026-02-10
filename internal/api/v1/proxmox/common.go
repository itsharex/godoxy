package proxmoxapi

type ActionRequest struct {
	Node string `uri:"node" binding:"required"`
	VMID uint64 `uri:"vmid" binding:"required"`
} //	@name	ProxmoxVMActionRequest
