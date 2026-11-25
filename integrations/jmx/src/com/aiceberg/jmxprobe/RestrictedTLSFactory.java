package com.aiceberg.jmxprobe;

import java.io.ByteArrayInputStream;
import java.io.FilterInputStream;
import java.io.IOException;
import java.io.InputStream;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.ServerSocket;
import java.net.Socket;
import java.nio.charset.StandardCharsets;
import java.rmi.server.RMISocketFactory;
import java.security.KeyStore;
import java.security.Security;
import java.security.cert.Certificate;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.Collection;
import java.util.concurrent.atomic.AtomicInteger;
import javax.net.ssl.SSLContext;
import javax.net.ssl.SSLParameters;
import javax.net.ssl.SSLSocket;
import javax.net.ssl.SSLSocketFactory;
import javax.net.ssl.TrustManagerFactory;

/** Used by the stock TLS socket factory in both the registry and JMX stubs. */
public final class RestrictedTLSFactory extends SSLSocketFactory {
    private static final int MAX_SOCKET_BYTES = 128 * 1024;
    private static final int MAX_SOCKETS = 6;
    private static final AtomicInteger opened = new AtomicInteger();
    private static volatile ProbeProfile profile;
    private static volatile SSLSocketFactory delegate;

    public RestrictedTLSFactory() {
    }

    static void configure(ProbeProfile approved) throws Exception {
        KeyStore store = KeyStore.getInstance(KeyStore.getDefaultType());
        store.load(null);
        Collection<? extends Certificate> anchors = CertificateFactory.getInstance("X.509").generateCertificates(
            new ByteArrayInputStream(approved.ca().getBytes(StandardCharsets.US_ASCII)));
        if (anchors.isEmpty() || anchors.size() > 8) {
            throw new IOException("invalid_ca_bundle");
        }
        int index = 0;
        for (Certificate certificate : anchors) {
            if (!(certificate instanceof X509Certificate issuer) || issuer.getBasicConstraints() < 0) {
                throw new IOException("invalid_ca_bundle");
            }
            store.setCertificateEntry("anchor-" + index++, issuer);
        }
        TrustManagerFactory trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        trust.init(store);
        SSLContext context = SSLContext.getInstance("TLS");
        context.init(null, trust.getTrustManagers(), null);
        delegate = context.getSocketFactory();
        profile = approved;
        Security.setProperty("ssl.SocketFactory.provider", RestrictedTLSFactory.class.getName());
        if (!(SSLSocketFactory.getDefault() instanceof RestrictedTLSFactory)) {
            throw new IOException("restricted_tls_unavailable");
        }
        RMISocketFactory.setSocketFactory(new RMISocketFactory() {
            @Override public Socket createSocket(String host, int port) throws IOException { throw denied(); }
            @Override public ServerSocket createServerSocket(int port) throws IOException { throw denied(); }
        });
    }

    @Override public Socket createSocket(String host, int port) throws IOException {
        ProbeProfile target = profile;
        if (target == null || !target.host().equalsIgnoreCase(host)
            || (port != target.registryPort() && port != target.serverPort())
            || opened.incrementAndGet() > MAX_SOCKETS) {
            throw denied();
        }
        BoundedSocket raw = new BoundedSocket();
        try {
            raw.connect(new InetSocketAddress(target.address(), port), target.budgetMs());
            raw.setSoTimeout(target.budgetMs());
            SSLSocket secure = (SSLSocket) delegate.createSocket(raw, target.host(), port, true);
            SSLParameters policy = secure.getSSLParameters();
            policy.setProtocols(new String[]{"TLSv1.3", "TLSv1.2"});
            policy.setEndpointIdentificationAlgorithm("HTTPS");
            secure.setSSLParameters(policy);
            return secure;
        } catch (IOException | RuntimeException error) {
            raw.close();
            throw error;
        }
    }

    @Override public Socket createSocket() throws IOException { throw denied(); }
    @Override public Socket createSocket(Socket socket, String host, int port, boolean autoClose) throws IOException { throw denied(); }
    @Override public Socket createSocket(String host, int port, InetAddress local, int localPort) throws IOException { throw denied(); }
    @Override public Socket createSocket(InetAddress host, int port) throws IOException { throw denied(); }
    @Override public Socket createSocket(InetAddress host, int port, InetAddress local, int localPort) throws IOException { throw denied(); }
    @Override public String[] getDefaultCipherSuites() { return delegate.getDefaultCipherSuites(); }
    @Override public String[] getSupportedCipherSuites() { return delegate.getSupportedCipherSuites(); }

    private static IOException denied() { return new DisallowedDestination(); }

    static final class DisallowedDestination extends IOException {
        DisallowedDestination() { super("rmi_destination_not_allowed"); }
    }

    private static final class BoundedSocket extends Socket {
        private InputStream bounded;

        @Override public synchronized InputStream getInputStream() throws IOException {
            if (bounded == null) {
                // Both TLS negotiation and RMI payload count against the same socket limit.
                bounded = new FilterInputStream(super.getInputStream()) {
                    private int remaining = MAX_SOCKET_BYTES;
                    @Override public int read() throws IOException {
                        ensureAvailable();
                        int value = in.read();
                        if (value >= 0) {
                            remaining--;
                        }
                        return value;
                    }
                    @Override public int read(byte[] bytes, int offset, int length) throws IOException {
                        if (length == 0) {
                            return 0;
                        }
                        ensureAvailable();
                        int count = in.read(bytes, offset, Math.min(length, remaining));
                        if (count > 0) {
                            remaining -= count;
                        }
                        return count;
                    }
                    @Override public long skip(long count) throws IOException {
                        if (count <= 0) {
                            return 0;
                        }
                        ensureAvailable();
                        long skipped = in.skip(Math.min(count, remaining));
                        remaining -= (int) skipped;
                        return skipped;
                    }
                    @Override public int available() throws IOException {
                        return Math.min(in.available(), remaining);
                    }
                    @Override public boolean markSupported() { return false; }
                    @Override public synchronized void mark(int limit) { }
                    @Override public synchronized void reset() throws IOException { throw new IOException("rmi_socket_reset"); }
                    private void ensureAvailable() throws IOException {
                        if (remaining == 0) {
                            throw new IOException("rmi_socket_limit");
                        }
                    }
                };
            }
            return bounded;
        }
    }
}
