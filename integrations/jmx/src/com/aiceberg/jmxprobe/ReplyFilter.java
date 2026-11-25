package com.aiceberg.jmxprobe;

import java.io.ObjectInputFilter;
import java.util.Set;

final class ReplyFilter {
    private static final Set<String> ALLOWED_TYPES = Set.of(
        "javax.management.remote.rmi.RMIServerImpl_Stub",
        "javax.management.remote.rmi.RMIConnectionImpl_Stub",
        "javax.rmi.ssl.SslRMIClientSocketFactory",
        "java.rmi.server.RemoteStub", "java.rmi.server.RemoteObject",
        "java.rmi.server.ObjID", "java.rmi.server.UID",
        "sun.rmi.transport.DGCImpl_Stub", "java.rmi.dgc.Lease", "java.rmi.dgc.VMID",
        "java.lang.String", "java.lang.Long", "java.lang.Integer", "java.lang.Number",
        "javax.management.openmbean.CompositeDataSupport", "javax.management.openmbean.CompositeType",
        "javax.management.openmbean.OpenType", "javax.management.openmbean.SimpleType",
        "java.util.TreeMap"
    );

    private ReplyFilter() {
    }

    static void install() {
        ObjectInputFilter limits = ReplyFilter::check;
        ObjectInputFilter.Config.setSerialFilterFactory((current, requested) -> {
            ObjectInputFilter nested = current == null ? requested
                : (requested == null ? current : ObjectInputFilter.merge(current, requested));
            return nested == null ? limits : ObjectInputFilter.merge(limits, nested);
        });
    }

    private static ObjectInputFilter.Status check(ObjectInputFilter.FilterInfo info) {
        if (info.depth() > 16 || info.references() > 256 || info.arrayLength() > 64 || info.streamBytes() > 64 * 1024) {
            return ObjectInputFilter.Status.REJECTED;
        }
        Class<?> type = info.serialClass();
        if (type == null) {
            return ObjectInputFilter.Status.UNDECIDED;
        }
        while (type.isArray()) {
            type = type.getComponentType();
        }
        return type.isPrimitive() || ALLOWED_TYPES.contains(type.getName())
            ? ObjectInputFilter.Status.ALLOWED : ObjectInputFilter.Status.REJECTED;
    }
}
