#include "shim.h"

enum ZT_ResultCode zt_node_new(
	ZT_Node **node,
	const struct ZT_Node_Config *config,
	uintptr_t handle,
	void *tptr,
	const struct ZT_Node_Callbacks *callbacks,
	int64_t now)
{
	return ZT_Node_new(node, config, (void *)handle, tptr, callbacks, now);
}

static int shim_virtualNetworkConfigFunction(
	ZT_Node *node, void *uptr, void *tptr,
	uint64_t nwid, void **nuptr,
	enum ZT_VirtualNetworkConfigOperation op,
	const ZT_VirtualNetworkConfig *config)
{
	return goVirtualNetworkConfigCallback((uintptr_t)uptr, nwid, (int)op, (ZT_VirtualNetworkConfig *)config);
}

static void shim_virtualNetworkFrameFunction(
	ZT_Node *node, void *uptr, void *tptr,
	uint64_t nwid, void **nuptr,
	uint64_t sourceMac, uint64_t destMac,
	unsigned int etherType, unsigned int vlanId,
	const void *data, unsigned int len)
{
	goVirtualNetworkFrameCallback((uintptr_t)uptr, nwid, sourceMac, destMac, etherType, vlanId, (void *)data, len);
}

static void shim_eventCallback(
	ZT_Node *node, void *uptr, void *tptr,
	enum ZT_Event event, const void *data)
{
	goEventCallback((uintptr_t)uptr, (int)event, (void *)data);
}

static void shim_statePutFunction(
	ZT_Node *node, void *uptr, void *tptr,
	enum ZT_StateObjectType type, const uint64_t id[2],
	const void *data, int len)
{
	goStatePutCallback((uintptr_t)uptr, (int)type, id[0], id[1], (void *)data, len);
}

static int shim_stateGetFunction(
	ZT_Node *node, void *uptr, void *tptr,
	enum ZT_StateObjectType type, const uint64_t id[2],
	void *buf, unsigned int bufLen)
{
	return goStateGetCallback((uintptr_t)uptr, (int)type, id[0], id[1], buf, bufLen);
}

static int shim_wirePacketSendFunction(
	ZT_Node *node, void *uptr, void *tptr,
	int64_t localSocket,
	const struct sockaddr_storage *remoteAddress,
	const void *data, unsigned int len, unsigned int ttl)
{
	return goWirePacketSendCallback((uintptr_t)uptr, localSocket,
		(struct sockaddr_storage *)remoteAddress, (void *)data, len, ttl);
}

static int shim_pathCheckFunction(
	ZT_Node *node, void *uptr, void *tptr,
	uint64_t ztAddress, int64_t localSocket,
	const struct sockaddr_storage *remoteAddress)
{
	return goPathCheckCallback((uintptr_t)uptr, ztAddress, localSocket,
		(struct sockaddr_storage *)remoteAddress);
}

static int shim_pathLookupFunction(
	ZT_Node *node, void *uptr, void *tptr,
	uint64_t ztAddress, int desiredAddressFamily,
	struct sockaddr_storage *result)
{
	return goPathLookupCallback((uintptr_t)uptr, ztAddress, desiredAddressFamily, result);
}

void zt_init_callbacks(struct ZT_Node_Callbacks *cb)
{
	cb->version = 0;
	cb->statePutFunction = shim_statePutFunction;
	cb->stateGetFunction = shim_stateGetFunction;
	cb->wirePacketSendFunction = shim_wirePacketSendFunction;
	cb->virtualNetworkFrameFunction = shim_virtualNetworkFrameFunction;
	cb->virtualNetworkConfigFunction = shim_virtualNetworkConfigFunction;
	cb->eventCallback = shim_eventCallback;
	cb->pathCheckFunction = shim_pathCheckFunction;
	cb->pathLookupFunction = shim_pathLookupFunction;
}
