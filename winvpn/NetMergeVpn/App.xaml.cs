// The small window NetMerge opens to set up and switch the "NetMerge" Windows VPN (test). NetMerge starts it with a
// link: netmerge-vpn:connect?cfg=<base64 of the plug-in settings>, or netmerge-vpn:disconnect. The result is written
// to status.txt in this app's LocalState folder, where NetMerge reads it, and the window closes.
using System;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading.Tasks;
using Windows.ApplicationModel;
using Windows.ApplicationModel.Activation;
using Windows.Networking.Vpn;
using Windows.Storage;
using Windows.UI.Xaml;
using Windows.UI.Xaml.Controls;

namespace NetMergeVpn
{
    sealed partial class App : Application
    {
        const string ProfileName = "NetMerge";
        TextBlock text;

        public App()
        {
            InitializeComponent();
        }

        protected override void OnLaunched(LaunchActivatedEventArgs e)
        {
            Show("NetMerge Windows VPN (test)\n\nThis helper is used by NetMerge. Turn the test on in NetMerge: Settings, How NetMerge catches traffic.");
        }

        protected override async void OnActivated(IActivatedEventArgs args)
        {
            Show("NetMerge: switching the Windows VPN…");
            string result;
            try
            {
                var uri = (args as ProtocolActivatedEventArgs)?.Uri;
                var action = uri?.AbsolutePath?.Trim('/') ?? "";
                if (action == "connect")
                {
                    var q = uri.Query.TrimStart('?').Split('&').Select(p => p.Split(new[] { '=' }, 2))
                        .Where(p => p.Length == 2).ToDictionary(p => p[0], p => Uri.UnescapeDataString(p[1]));
                    var cfg = Encoding.UTF8.GetString(Convert.FromBase64String(q["cfg"]));
                    result = await Connect(cfg);
                }
                else if (action == "disconnect")
                {
                    result = await Disconnect();
                }
                else
                {
                    result = "unknown request: " + uri;
                }
            }
            catch (Exception ex)
            {
                result = "error: " + ex.Message;
            }
            await WriteStatus(result);
            Show("NetMerge: " + result);
            await Task.Delay(800);
            Exit();
        }

        void Show(string msg)
        {
            if (text == null)
            {
                text = new TextBlock { Margin = new Thickness(24), TextWrapping = TextWrapping.Wrap, FontSize = 16 };
                Window.Current.Content = new Grid { Children = { text } };
            }
            text.Text = msg;
            Window.Current.Activate();
        }

        static async Task<VpnPlugInProfile> Find(VpnManagementAgent agent)
        {
            var all = await agent.GetProfilesAsync();
            return all.OfType<VpnPlugInProfile>().FirstOrDefault(p => p.ProfileName == ProfileName);
        }

        static async Task<string> Connect(string cfg)
        {
            var agent = new VpnManagementAgent();
            var p = await Find(agent);
            var isNew = p == null;
            if (isNew)
            {
                p = new VpnPlugInProfile { ProfileName = ProfileName };
            }
            p.VpnPluginPackageFamilyName = Package.Current.Id.FamilyName;
            p.CustomConfiguration = cfg;
            p.RememberCredentials = false;
            p.RequireVpnClientAppUI = false;
            p.AlwaysOn = false;
            if (p.ServerUris.Count == 0)
            {
                p.ServerUris.Add(new Uri("http://127.0.0.1"));
            }
            var st = isNew ? await agent.AddProfileFromObjectAsync(p) : await agent.UpdateProfileFromObjectAsync(p);
            if (st != VpnManagementErrorStatus.Ok)
            {
                return "could not save the VPN profile: " + st;
            }
            p = await Find(agent);
            if (p == null)
            {
                return "the VPN profile was saved but cannot be found";
            }
            var c = await agent.ConnectProfileAsync(p);
            return c == VpnManagementErrorStatus.Ok ? "connected" : "could not connect: " + c;
        }

        static async Task<string> Disconnect()
        {
            var agent = new VpnManagementAgent();
            var p = await Find(agent);
            if (p == null)
            {
                return "disconnected (no profile)";
            }
            var st = await agent.DisconnectProfileAsync(p);
            return st == VpnManagementErrorStatus.Ok ? "disconnected" : "could not disconnect: " + st;
        }

        static async Task WriteStatus(string s)
        {
            try
            {
                var f = await ApplicationData.Current.LocalFolder.CreateFileAsync("status.txt", CreationCollisionOption.ReplaceExisting);
                await FileIO.WriteTextAsync(f, DateTimeOffset.Now.ToString("o") + " " + s);
            }
            catch
            {
            }
        }
    }
}
