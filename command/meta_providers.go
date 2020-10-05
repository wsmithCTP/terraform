package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	hclog "github.com/hashicorp/go-hclog"
	plugin "github.com/hashicorp/go-plugin"

	"github.com/hashicorp/terraform/addrs"
	terraformProvider "github.com/hashicorp/terraform/builtin/providers/terraform"
	"github.com/hashicorp/terraform/internal/getproviders"
	"github.com/hashicorp/terraform/internal/providercache"
	tfplugin "github.com/hashicorp/terraform/plugin"
	"github.com/hashicorp/terraform/providers"
)

// The TF_DISABLE_PLUGIN_TLS environment variable is intended only for use by
// the plugin SDK test framework, to reduce startup overhead when rapidly
// launching and killing lots of instances of the same provider.
//
// This is not intended to be set by end-users.
var enableProviderAutoMTLS = os.Getenv("TF_DISABLE_PLUGIN_TLS") == ""

// providerInstaller returns an object that knows how to install providers and
// how to recover the selections from a prior installation process.
//
// The resulting provider installer is constructed from the results of
// the other methods providerLocalCacheDir, providerGlobalCacheDir, and
// providerInstallSource.
//
// Only one object returned from this method should be live at any time,
// because objects inside contain caches that must be maintained properly.
// Because this method wraps a result from providerLocalCacheDir, that
// limitation applies also to results from that method.
func (m *Meta) providerInstaller() *providercache.Installer {
	return m.providerInstallerCustomSource(m.providerInstallSource())
}

// providerInstallerCustomSource is a variant of providerInstaller that
// allows the caller to specify a different installation source than the one
// that would naturally be selected.
//
// The result of this method has the same dependencies and constraints as
// providerInstaller.
//
// The result of providerInstallerCustomSource differs from
// providerInstaller only in how it determines package installation locations
// during EnsureProviderVersions. A caller that doesn't call
// EnsureProviderVersions (anything other than "terraform init") can safely
// just use the providerInstaller method unconditionally.
func (m *Meta) providerInstallerCustomSource(source getproviders.Source) *providercache.Installer {
	targetDir := m.providerLocalCacheDir()
	globalCacheDir := m.providerGlobalCacheDir()
	inst := providercache.NewInstaller(targetDir, source)
	if globalCacheDir != nil {
		inst.SetGlobalCacheDir(globalCacheDir)
	}
	var builtinProviderTypes []string
	for ty := range m.internalProviders() {
		builtinProviderTypes = append(builtinProviderTypes, ty)
	}
	inst.SetBuiltInProviderTypes(builtinProviderTypes)
	unmanagedProviderTypes := make(map[addrs.Provider]struct{}, len(m.UnmanagedProviders))
	for ty := range m.UnmanagedProviders {
		unmanagedProviderTypes[ty] = struct{}{}
	}
	inst.SetUnmanagedProviderTypes(unmanagedProviderTypes)
	return inst
}

// providerCustomLocalDirectorySource produces a provider source that consults
// only the given local filesystem directories for plugins to install.
//
// This is used to implement the -plugin-dir option for "terraform init", where
// the result of this method is used instead of what would've been returned
// from m.providerInstallSource.
//
// If the given list of directories is empty then the resulting source will
// have no providers available for installation at all.
func (m *Meta) providerCustomLocalDirectorySource(dirs []string) getproviders.Source {
	var ret getproviders.MultiSource
	for _, dir := range dirs {
		ret = append(ret, getproviders.MultiSourceSelector{
			Source: getproviders.NewFilesystemMirrorSource(dir),
		})
	}
	return ret
}

// providerLocalCacheDir returns an object representing the
// configuration-specific local cache directory. This is the
// only location consulted for provider plugin packages for Terraform
// operations other than provider installation.
//
// Only the provider installer (in "terraform init") is permitted to make
// modifications to this cache directory. All other commands must treat it
// as read-only.
//
// Only one object returned from this method should be live at any time,
// because objects inside contain caches that must be maintained properly.
func (m *Meta) providerLocalCacheDir() *providercache.Dir {
	dir := filepath.Join(m.DataDir(), "plugins")
	if dir == "" {
		return nil // cache disabled
	}
	return providercache.NewDir(dir)
}

// providerGlobalCacheDir returns an object representing the shared global
// provider cache directory, used as a read-through cache when installing
// new provider plugin packages.
//
// This function may return nil, in which case there is no global cache
// configured and new packages should be downloaded directly into individual
// configuration-specific cache directories.
//
// Only one object returned from this method should be live at any time,
// because objects inside contain caches that must be maintained properly.
func (m *Meta) providerGlobalCacheDir() *providercache.Dir {
	dir := m.PluginCacheDir
	if dir == "" {
		return nil // cache disabled
	}
	return providercache.NewDir(dir)
}

// providerInstallSource returns an object that knows how to consult one or
// more external sources to determine the availability of and package
// locations for versions of Terraform providers that are available for
// automatic installation.
//
// This returns the standard provider install source that consults a number
// of directories selected either automatically or via the CLI configuration.
// Users may choose to override this during a "terraform init" command by
// specifying one or more -plugin-dir options, in which case the installation
// process will construct its own source consulting only those directories
// and use that instead.
func (m *Meta) providerInstallSource() getproviders.Source {
	// A provider source should always be provided in normal use, but our
	// unit tests might not always populate Meta fully and so we'll be robust
	// by returning a non-nil source that just always answers that no plugins
	// are available.
	if m.ProviderSource == nil {
		// A multi-source with no underlying sources is effectively an
		// always-empty source.
		return getproviders.MultiSource(nil)
	}
	return m.ProviderSource
}

// providerFactories uses the selections made previously by an installer in
// the local cache directory (m.providerLocalCacheDir) to produce a map
// from provider addresses to factory functions to create instances of
// those providers.
//
// providerFactories will return an error if the installer's selections cannot
// be honored with what is currently in the cache, such as if a selected
// package has been removed from the cache or if the contents of a selected
// package have been modified outside of the installer. If it returns an error,
// the returned map may be incomplete or invalid.
func (m *Meta) providerFactories() (map[addrs.Provider]providers.Factory, error) {
	// TODO: Update this to use the dependency lock file instead of the
	// old "SelectedPackages" concept. We should expect to find all of the
	// locked providers in our cache directory because a previous run of
	// "terraform init" should've put them there.
	return nil, fmt.Errorf("providerFactories is not yet updated to use the lock file")

	/*
		// We don't have to worry about potentially calling
		// providerInstallerCustomSource here because we're only using this
		// installer for its SelectedPackages method, which does not consult
		// any provider sources.
		inst := m.providerInstaller()
		selected, err := inst.SelectedPackages()
		if err != nil {
			return nil, fmt.Errorf("failed to recall provider packages selected by earlier 'terraform init': %s", err)
		}

		// The internal providers are _always_ available, even if the configuration
		// doesn't request them, because they don't need any special installation
		// and they'll just be ignored if not used.
		internalFactories := m.internalProviders()

		factories := make(map[addrs.Provider]providers.Factory, len(selected)+len(internalFactories)+len(m.UnmanagedProviders))
		for name, factory := range internalFactories {
			factories[addrs.NewBuiltInProvider(name)] = factory
		}
		for provider, reattach := range m.UnmanagedProviders {
			factories[provider] = unmanagedProviderFactory(provider, reattach)
		}
		for provider, cached := range selected {
			factories[provider] = providerFactory(cached)
		}
		return factories, nil
	*/
}

func (m *Meta) internalProviders() map[string]providers.Factory {
	return map[string]providers.Factory{
		"terraform": func() (providers.Interface, error) {
			return terraformProvider.NewProvider(), nil
		},
	}
}

// providerFactory produces a provider factory that runs up the executable
// file in the given cache package and uses go-plugin to implement
// providers.Interface against it.
func providerFactory(meta *providercache.CachedProvider) providers.Factory {
	return func() (providers.Interface, error) {
		logger := hclog.New(&hclog.LoggerOptions{
			Name:   "plugin",
			Level:  hclog.Trace,
			Output: os.Stderr,
		})

		execFile, err := meta.ExecutableFile()
		if err != nil {
			return nil, err
		}

		config := &plugin.ClientConfig{
			HandshakeConfig:  tfplugin.Handshake,
			Logger:           logger,
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Managed:          true,
			Cmd:              exec.Command(execFile),
			AutoMTLS:         enableProviderAutoMTLS,
			VersionedPlugins: tfplugin.VersionedPlugins,
		}

		client := plugin.NewClient(config)
		rpcClient, err := client.Client()
		if err != nil {
			return nil, err
		}

		raw, err := rpcClient.Dispense(tfplugin.ProviderPluginName)
		if err != nil {
			return nil, err
		}

		// store the client so that the plugin can kill the child process
		p := raw.(*tfplugin.GRPCProvider)
		p.PluginClient = client

		return p, nil
	}
}

// unmanagedProviderFactory produces a provider factory that uses the passed
// reattach information to connect to go-plugin processes that are already
// running, and implements providers.Interface against it.
func unmanagedProviderFactory(provider addrs.Provider, reattach *plugin.ReattachConfig) providers.Factory {
	return func() (providers.Interface, error) {
		logger := hclog.New(&hclog.LoggerOptions{
			Name:   "unmanaged-plugin",
			Level:  hclog.Trace,
			Output: os.Stderr,
		})

		config := &plugin.ClientConfig{
			HandshakeConfig:  tfplugin.Handshake,
			Logger:           logger,
			AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
			Managed:          false,
			Reattach:         reattach,
		}
		// TODO: we probably shouldn't hardcode the protocol version
		// here, but it'll do for now, because only one protocol
		// version is supported. Eventually, we'll probably want to
		// sneak it into the JSON ReattachConfigs.
		if plugins, ok := tfplugin.VersionedPlugins[5]; !ok {
			return nil, fmt.Errorf("no supported plugins for protocol 5")
		} else {
			config.Plugins = plugins
		}

		client := plugin.NewClient(config)
		rpcClient, err := client.Client()
		if err != nil {
			return nil, err
		}

		raw, err := rpcClient.Dispense(tfplugin.ProviderPluginName)
		if err != nil {
			return nil, err
		}

		p := raw.(*tfplugin.GRPCProvider)
		return p, nil
	}
}
