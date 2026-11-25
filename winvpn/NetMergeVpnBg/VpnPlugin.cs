// NetMerge's Windows VPN plug-in (test). Windows' own VPN platform gives this plug-in every IP packet the PC sends
// through the "NetMerge" VPN; the plug-in passes each packet unchanged to the NetMerge engine on this PC
// (127.0.0.1, UDP), and passes the engine's packets back to Windows. There is no VPN server and no encryption: the
// traffic never leaves the PC here. The engine then sends it out over the lines, as it does with its own adapter.
//
// Packets on the loopback socket: one IP packet per datagram. A datagram starting with a zero byte is a control
// message (an IP packet never starts with 0): "\0NMVPN1 <token>" tells the engine which socket is the plug-in.

using System;
using System.Collections.Generic;
using System.Linq;
using System.Runtime.InteropServices.WindowsRuntime;
using System.Text;
using System.Xml.Linq;
using Windows.Networking;
using Windows.Networking.Sockets;
using Windows.Networking.Vpn;
using Windows.Storage.Streams;

namespace NetMergeVpnBg
{
    public sealed class NetMergePlugin : IVpnPlugIn
    {
        DatagramSocket socket;
        byte[] hello = new byte[0];

        public void Connect(VpnChannel channel)
        {
            try
            {
                var cfg = XDocument.Parse(channel.Configuration.CustomField ?? "<nm/>").Root;
                string Get(string name, string def) => cfg.Element(name)?.Value ?? def;
                int port = int.Parse(Get("port", "0"));
                string token = Get("token", "");
                uint mtu = uint.Parse(Get("mtu", "1500"));
                string dns = Get("dns", "");
                if (port <= 0 || token.Length == 0)
                {
                    channel.TerminateConnection("NetMerge settings are missing in the VPN profile. Turn the Windows VPN test on again in NetMerge.");
                    return;
                }
                hello = Encoding.ASCII.GetBytes("\0NMVPN1 " + token);

                socket = new DatagramSocket();
                // a large receive buffer: at full speed the engine sends faster than one Decapsulate call at a time
                // takes them, and a lost packet makes TCP slow down
                socket.Control.InboundBufferSizeInBytes = 8 * 1024 * 1024;
                channel.AssociateTransport(socket, null);
                socket.ConnectAsync(new HostName("127.0.0.1"), port.ToString()).AsTask().Wait();
                SendHello();

                var routes = new VpnRouteAssignment();
                // everything, unless the settings name the ranges to catch ("a.b.c.d/n,..."; the automatic test
                // on GitHub uses this so the machine's own connection to GitHub stays outside the VPN)
                var only = Get("routes", "");
                var included = new List<VpnRoute>();
                foreach (var r in only.Split(new[] { ',' }, StringSplitOptions.RemoveEmptyEntries))
                {
                    var parts = r.Trim().Split('/');
                    included.Add(new VpnRoute(new HostName(parts[0]), parts.Length > 1 ? byte.Parse(parts[1]) : (byte)32));
                }
                if (included.Count == 0)
                {
                    included.Add(new VpnRoute(new HostName("0.0.0.0"), 1));
                    included.Add(new VpnRoute(new HostName("128.0.0.0"), 1));
                }
                else
                {
                    // NetMerge's own range: the engine answers apps from 198.18.0.2 (and DNS there), so those replies
                    // must go back into the VPN too (with everything routed this is already the case)
                    included.Add(new VpnRoute(new HostName("198.18.0.0"), 16));
                }
                routes.Ipv4InclusionRoutes = included;
                routes.ExcludeLocalSubnets = true;

                VpnDomainNameAssignment names = null;
                if (dns.Length > 0)
                {
                    names = new VpnDomainNameAssignment();
                    names.DomainNameList.Add(new VpnDomainNameInfo(".", VpnDomainNameType.Suffix,
                        new List<HostName> { new HostName(dns) }, new List<HostName>()));
                }

                channel.StartWithMainTransport(
                    new List<HostName> { new HostName("198.18.0.1") }, // the address NetMerge's engine expects apps to use
                    null,       // no IPv6 (NetMerge leaves IPv6 out, as with its own adapter)
                    null,
                    routes,
                    names,
                    mtu,
                    mtu + 64,   // largest datagram on the loopback socket: one packet
                    false,
                    socket);
            }
            catch (Exception ex)
            {
                channel.TerminateConnection("NetMerge VPN could not start: " + ex.Message);
            }
        }

        void SendHello()
        {
            var w = new DataWriter(socket.OutputStream);
            w.WriteBytes(hello);
            w.StoreAsync().AsTask().Wait();
            w.DetachStream();
        }

        public void Disconnect(VpnChannel channel)
        {
            try { channel.Stop(); } catch { }
            try { socket?.Dispose(); } catch { }
            socket = null;
        }

        // Windows hands over the packets apps sent; they go to the engine as they are.
        public void Encapsulate(VpnChannel channel, VpnPacketBufferList packets, VpnPacketBufferList encapulatedPackets)
        {
            while (packets.Size > 0)
            {
                encapulatedPackets.Append(packets.RemoveAtBegin());
            }
        }

        // A datagram from the engine is one IP packet for the PC (control messages from the engine are ignored).
        public void Decapsulate(VpnChannel channel, VpnPacketBuffer encapBuffer, VpnPacketBufferList decapsulatedPackets, VpnPacketBufferList controlPacketsToSend)
        {
            var src = encapBuffer.Buffer;
            if (src.Length == 0)
            {
                return;
            }
            if (src.GetByte(0) == 0)
            {
                return;
            }
            VpnPacketBuffer pkt;
            channel.RequestVpnPacketBuffer(VpnDataPathType.Receive, out pkt);
            if (pkt.Buffer.Capacity < src.Length)
            {
                return; // larger than the adapter allows: dropped, TCP adapts its size
            }
            src.CopyTo(0, pkt.Buffer, 0, src.Length);
            pkt.Buffer.Length = src.Length;
            decapsulatedPackets.Append(pkt);
        }

        // Windows asks for a keep-alive now and then: it doubles as the hello, so a restarted engine learns the
        // plug-in's socket again without the VPN reconnecting.
        public void GetKeepAlivePayload(VpnChannel channel, out VpnPacketBuffer keepAlivePacket)
        {
            keepAlivePacket = null;
            if (hello.Length == 0)
            {
                return;
            }
            channel.RequestVpnPacketBuffer(VpnDataPathType.Send, out keepAlivePacket);
            hello.CopyTo(0, keepAlivePacket.Buffer, 0, hello.Length);
            keepAlivePacket.Buffer.Length = (uint)hello.Length;
        }
    }
}
