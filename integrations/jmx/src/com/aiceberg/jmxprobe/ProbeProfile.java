package com.aiceberg.jmxprobe;

import java.io.DataInputStream;
import java.io.IOException;
import java.net.InetAddress;
import java.nio.charset.StandardCharsets;
import java.util.regex.Pattern;

record ProbeProfile(String host, InetAddress address, int registryPort, int serverPort,
                    int budgetMs, String user, String password, String ca) {
    private static final Pattern DNS_NAME = Pattern.compile(
        "[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*");
    private static final Pattern IPV4 = Pattern.compile("[0-9]{1,3}(?:\\.[0-9]{1,3}){3}");
    private static final Pattern IPV6 = Pattern.compile("[0-9a-fA-F:.]+");

    static ProbeProfile read(DataInputStream input) throws IOException {
        if (input.readInt() != 0x41494a31) {
            throw new IOException("profile_version");
        }
        String host = text(input, 253);
        InetAddress address = numericAddress(text(input, 64));
        int registry = input.readUnsignedShort();
        int server = input.readUnsignedShort();
        int budget = input.readInt();
        String user = text(input, 64);
        String password = text(input, 1024);
        String ca = text(input, 32768);
        if ((!DNS_NAME.matcher(host).matches() && !address.equals(numericAddress(host)))
            || registry == 0 || server == 0 || budget < 1000 || budget > 20000
            || !user.matches("[A-Za-z_][A-Za-z0-9_.-]{0,63}") || password.indexOf('\u0000') >= 0
            || input.read() != -1) {
            throw new IOException("profile_invalid");
        }
        if (IPV4.matcher(host).matches() || host.indexOf(':') >= 0) {
            if (!address.equals(numericAddress(host))) {
                throw new IOException("profile_ip_mismatch");
            }
        } else if (address.isLoopbackAddress()) {
            throw new IOException("profile_dns_loopback");
        }
        return new ProbeProfile(host, address, registry, server, budget, user, password, ca);
    }

    @Override public String toString() { return "JMX probe profile"; }

    private static String text(DataInputStream input, int max) throws IOException {
        int length = input.readUnsignedShort();
        if (length < 1 || length > max) {
            throw new IOException("profile_size");
        }
        byte[] bytes = new byte[length];
        input.readFully(bytes);
        return new String(bytes, StandardCharsets.UTF_8);
    }

    private static InetAddress numericAddress(String text) throws IOException {
        InetAddress address;
        if (IPV4.matcher(text).matches()) {
            String[] components = text.split("\\.");
            byte[] bytes = new byte[4];
            for (int index = 0; index < components.length; index++) {
                int part = Integer.parseInt(components[index]);
                if (part > 255 || !components[index].equals(Integer.toString(part))) {
                    throw new IOException("profile_ip_invalid");
                }
                bytes[index] = (byte) part;
            }
            address = InetAddress.getByAddress(bytes);
        } else if (text.indexOf(':') >= 0 && IPV6.matcher(text).matches()) {
            address = InetAddress.getByName(text);
        } else {
            throw new IOException("profile_ip_required");
        }
        if (address.isAnyLocalAddress() || address.isLinkLocalAddress() || address.isMulticastAddress()
            || address.getHostAddress().equals("100.100.100.200")
            || address.equals(InetAddress.getByName("fd00:ec2::254"))) {
            throw new IOException("profile_ip_not_allowed");
        }
        return address;
    }
}
