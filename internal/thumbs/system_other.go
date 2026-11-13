// Platforms without a supported system thumbnailer (currently Windows and
// the BSDs) get none: only natively decoded formats have previews and every
// other file shows its file-type icon. Windows shell thumbnails need COM
// (IShellItemImageFactory) and can be added here later without changing the
// rest of the package.

//go:build !darwin && !linux

package thumbs

func systemThumbnailer() SystemFunc { return nil }
