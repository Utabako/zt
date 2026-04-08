#ifndef ZT_GO_SHIM_H
#define ZT_GO_SHIM_H

#include <stdbool.h>
#include "ZeroTierOne.h"

// Forward declarations of Go-exported callback functions.
// These are implemented in callbacks.go via //export directives.
extern void goStatePutCallback(void *uptr, int objType, uint64_t id0, uint64_t id1, void *data, int dataLen);
extern int goStateGetCallback(void *uptr, int objType, uint64_t id0, uint64_t id1, void *buf, unsigned int bufLen);
extern int goWirePacketSendCallback(void *uptr, int64_t localSocket, struct sockaddr_storage *remoteAddr, void *data, unsigned int dataLen, unsigned int ttl);
extern void goVirtualNetworkFrameCallback(void *uptr, uint64_t nwid, uint64_t sourceMac, uint64_t destMac, unsigned int etherType, unsigned int vlanID, void *data, unsigned int dataLen);
extern int goVirtualNetworkConfigCallback(void *uptr, uint64_t nwid, int op, ZT_VirtualNetworkConfig *config);
extern void goEventCallback(void *uptr, int event, void *data);
extern int goPathCheckCallback(void *uptr, uint64_t ztAddress, int64_t localSocket, struct sockaddr_storage *remoteAddr);
extern int goPathLookupCallback(void *uptr, uint64_t ztAddress, int desiredAddressFamily, struct sockaddr_storage *result);

// zt_init_callbacks initializes a ZT_Node_Callbacks struct with
// C shim functions that dispatch to Go exported callbacks.
void zt_init_callbacks(struct ZT_Node_Callbacks *cb);

#endif
