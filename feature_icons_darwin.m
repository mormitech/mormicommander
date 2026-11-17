// System file icons on macOS (the Go half is feature_icons_darwin.go): the icon NSWorkspace gives a file, or a file
// type by its extension ("/" stands for a plain folder), drawn at px × px into a PNG. Compiled without ARC; everything
// is made inside an autorelease pool, and the PNG is copied into malloc'd memory the Go side frees.

#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include <string.h>

void *mcIconPNG(const char *path, const char *ext, int px, int *outLen) {
	*outLen = 0;
	void *out = NULL;
	@autoreleasepool {
		NSWorkspace *ws = [NSWorkspace sharedWorkspace];
		NSImage *img = nil;
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
		if (path) {
			img = [ws iconForFile:[NSString stringWithUTF8String:path]];
		} else if (ext) {
			NSString *e = [NSString stringWithUTF8String:ext];
			if ([e isEqualToString:@"/"]) img = [ws iconForFileType:NSFileTypeForHFSTypeCode(kGenericFolderIcon)];
			else if ([e length] == 0) img = [ws iconForFileType:NSFileTypeForHFSTypeCode(kGenericDocumentIcon)];
			else img = [ws iconForFileType:e];
		}
#pragma clang diagnostic pop
		if (!img || px <= 0) return NULL;
		NSBitmapImageRep *rep = [[[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:px pixelsHigh:px
			bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSCalibratedRGBColorSpace
			bytesPerRow:0 bitsPerPixel:0] autorelease];
		if (!rep) return NULL;
		NSGraphicsContext *ctx = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
		[NSGraphicsContext saveGraphicsState];
		[NSGraphicsContext setCurrentContext:ctx];
		[img drawInRect:NSMakeRect(0, 0, px, px) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1.0];
		[NSGraphicsContext restoreGraphicsState];
		NSData *png = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (!png || [png length] == 0) return NULL;
		out = malloc([png length]);
		if (!out) return NULL;
		memcpy(out, [png bytes], [png length]);
		*outLen = (int)[png length];
	}
	return out;
}
