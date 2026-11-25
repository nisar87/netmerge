// Windows starts this task whenever the VPN needs the plug-in (connect, packets). The same plug-in object is used
// every time, so it keeps its socket between calls.
using System.Collections.Generic;
using Windows.ApplicationModel.Background;
using Windows.ApplicationModel.Core;
using Windows.Networking.Vpn;

namespace NetMergeVpnBg
{
    public sealed class VpnBackgroundTask : IBackgroundTask
    {
        const string key = "NetMergePlugin";

        public void Run(IBackgroundTaskInstance taskInstance)
        {
            var deferral = taskInstance.GetDeferral();
            try
            {
                var bag = (IDictionary<string, object>)CoreApplication.Properties;
                NetMergePlugin plugin;
                if (bag.ContainsKey(key))
                {
                    plugin = (NetMergePlugin)bag[key];
                }
                else
                {
                    plugin = new NetMergePlugin();
                    bag[key] = plugin;
                }
                VpnChannel.ProcessEventAsync(plugin, taskInstance.TriggerDetails);
            }
            catch
            {
            }
            finally
            {
                deferral.Complete();
            }
        }
    }
}
