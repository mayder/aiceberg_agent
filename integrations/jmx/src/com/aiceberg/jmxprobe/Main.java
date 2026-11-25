package com.aiceberg.jmxprobe;

import java.io.DataInputStream;
import java.io.InvalidClassException;
import java.net.SocketTimeoutException;
import java.rmi.registry.LocateRegistry;
import java.util.Map;
import java.util.StringJoiner;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import javax.management.MBeanServerConnection;
import javax.management.ObjectName;
import javax.management.openmbean.CompositeData;
import javax.management.remote.JMXConnector;
import javax.management.remote.rmi.RMIConnector;
import javax.management.remote.rmi.RMIServer;
import javax.net.ssl.SSLException;
import javax.rmi.ssl.SslRMIClientSocketFactory;

public final class Main {
    private static final AtomicBoolean reported = new AtomicBoolean();
    private Main() {
    }

    public static void main(String[] args) {
        ProbeProfile profile;
        try {
            if (args.length != 0) {
                throw new IllegalArgumentException("unexpected_arguments");
            }
            profile = ProbeProfile.read(new DataInputStream(System.in));
        } catch (Exception error) {
            fail("invalid_profile");
            return;
        }

        var watchdog = Executors.newSingleThreadScheduledExecutor();
        watchdog.schedule(() -> fail("timeout"), profile.budgetMs(), TimeUnit.MILLISECONDS);
        try {
            System.setProperty("java.rmi.server.useCodebaseOnly", "true");
            System.clearProperty("java.rmi.server.codebase");
            ReplyFilter.install();
            RestrictedTLSFactory.configure(profile);
            String metrics = collect(profile);
            report("{\"status\":\"ok\",\"metrics\":" + metrics + "}", 0);
        } catch (Exception error) {
            fail(safeError(error));
        }
    }

    private static String collect(ProbeProfile profile) throws Exception {
        var registry = LocateRegistry.getRegistry(profile.host(), profile.registryPort(), new SslRMIClientSocketFactory());
        var server = (RMIServer) registry.lookup("jmxrmi");
        var env = Map.of(JMXConnector.CREDENTIALS, new String[]{profile.user(), profile.password()});
        try (var connector = new RMIConnector(server, null)) {
            connector.connect(env);
            return readMetrics(connector.getMBeanServerConnection());
        }
    }

    private static String readMetrics(MBeanServerConnection connection) throws Exception {
        var values = new StringJoiner(",", "{", "}");
        var heap = (CompositeData) connection.getAttribute(new ObjectName("java.lang:type=Memory"), "HeapMemoryUsage");
        add(values, "heap_used_bytes", heap.get("used"));
        add(values, "heap_max_bytes", heap.get("max"));
        add(values, "uptime_ms", connection.getAttribute(new ObjectName("java.lang:type=Runtime"), "Uptime"));
        var threads = new ObjectName("java.lang:type=Threading");
        add(values, "live_threads", connection.getAttribute(threads, "ThreadCount"));
        add(values, "daemon_threads", connection.getAttribute(threads, "DaemonThreadCount"));
        add(values, "loaded_classes", connection.getAttribute(new ObjectName("java.lang:type=ClassLoading"), "LoadedClassCount"));
        return values.toString();
    }

    private static void add(StringJoiner values, String key, Object raw) {
        if (!(raw instanceof Long) && !(raw instanceof Integer)) {
            return;
        }
        long value = ((Number) raw).longValue();
        if (value >= 0 && value <= 9007199254740991L) {
            values.add("\"" + key + "\":" + value);
        }
    }

    private static String safeError(Throwable error) {
        Throwable cause = error;
        for (int depth = 0; depth < 16 && cause != null; depth++, cause = cause.getCause()) {
            if (cause instanceof RestrictedTLSFactory.DisallowedDestination) return "endpoint_not_allowed";
            if (cause instanceof InvalidClassException) return "response_rejected";
            if (cause instanceof SocketTimeoutException) return "timeout";
            if (cause instanceof SSLException) return "tls_failed";
            if (cause instanceof SecurityException) return "access_denied";
        }
        return "unavailable";
    }

    private static void fail(String reason) {
        report("{\"status\":\"fail\",\"code\":\"" + reason + "\"}", 1);
    }

    private static void report(String receipt, int exitCode) {
        if (reported.compareAndSet(false, true)) {
            System.out.println(receipt);
            System.exit(exitCode);
        }
    }
}
