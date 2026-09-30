#import <Cocoa/Cocoa.h>
#import <sys/socket.h>
#import <netinet/in.h>
#import <arpa/inet.h>
#import <unistd.h>

@interface AppDelegate : NSObject <NSApplicationDelegate>
@property (strong, nonatomic) NSStatusItem *statusItem;
@property (assign, nonatomic) NSInteger port;
@property (copy, nonatomic) NSString *dataDir;
@property (strong, nonatomic) NSTask *serverTask;
@end

@implementation AppDelegate

- (NSString *)configFilePath {
    NSString *home = NSHomeDirectory();
    NSString *dir = [home stringByAppendingPathComponent:@".slimbox"];
    [[NSFileManager defaultManager] createDirectoryAtPath:dir withIntermediateDirectories:YES attributes:nil error:nil];
    return [dir stringByAppendingPathComponent:@"config.json"];
}

- (void)loadConfig {
    self.port = 8080;
    NSString *home = NSHomeDirectory();
    self.dataDir = [home stringByAppendingPathComponent:@"Movies/SlimBox"];

    NSString *cfgPath = [self configFilePath];
    NSData *data = [NSData dataWithContentsOfFile:cfgPath];
    if (data) {
        NSError *err = nil;
        NSDictionary *dict = [NSJSONSerialization JSONObjectWithData:data options:0 error:&err];
        if (dict && [dict isKindOfClass:[NSDictionary class]]) {
            if (dict[@"port"] && [dict[@"port"] integerValue] > 0) {
                self.port = [dict[@"port"] integerValue];
            }
            if (dict[@"data_dir"] && [dict[@"data_dir"] length] > 0) {
                self.dataDir = dict[@"data_dir"];
            }
        }
    }
}

- (void)saveConfig {
    NSString *cfgPath = [self configFilePath];
    NSDictionary *dict = @{
        @"port": @(self.port),
        @"data_dir": self.dataDir ?: @""
    };
    NSData *data = [NSJSONSerialization dataWithJSONObject:dict options:NSJSONWritingPrettyPrinted error:nil];
    if (data) {
        [data writeToFile:cfgPath atomically:YES];
    }
}

- (BOOL)isPortListening:(NSInteger)port {
    int sock = socket(AF_INET, SOCK_STREAM, 0);
    if (sock < 0) return NO;
    
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    addr.sin_addr.s_addr = inet_addr("127.0.0.1");

    int res = connect(sock, (struct sockaddr *)&addr, sizeof(addr));
    close(sock);
    return (res == 0);
}

- (void)startServerIfNeeded {
    if ([self isPortListening:self.port]) {
        NSLog(@"[SlimBox Tray] Port %ld already listening, attaching to existing service...", (long)self.port);
        return;
    }

    NSString *bundlePath = [[NSBundle mainBundle] bundlePath];
    NSString *serverBin = [bundlePath stringByAppendingPathComponent:@"Contents/MacOS/slimbox-core"];
    
    if (![[NSFileManager defaultManager] isExecutableFileAtPath:serverBin]) {
        // Fallback to standalone location if running outside bundle
        serverBin = [bundlePath stringByAppendingPathComponent:@"slimbox"];
    }

    if (![[NSFileManager defaultManager] isExecutableFileAtPath:serverBin]) {
        NSLog(@"[SlimBox Tray] Warning: Server binary not found at %@", serverBin);
        return;
    }

    self.serverTask = [[NSTask alloc] init];
    [self.serverTask setExecutableURL:[NSURL fileURLWithPath:serverBin]];
    [self.serverTask setArguments:@[
        @"--port", [NSString stringWithFormat:@"%ld", (long)self.port],
        @"--data-dir", self.dataDir
    ]];

    NSError *err = nil;
    [self.serverTask launchAndReturnError:&err];
    if (err) {
        NSLog(@"[SlimBox Tray] Failed to launch serverTask: %@", err);
    } else {
        NSLog(@"[SlimBox Tray] Spawned server task PID %d", [self.serverTask processIdentifier]);
    }
}

- (void)applicationDidFinishLaunching:(NSNotification *)notification {
    [self loadConfig];
    [self startServerIfNeeded];

    // Create system status bar item (top menu bar)
    self.statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
    
    if (self.statusItem.button) {
        NSString *iconPath = [[NSBundle mainBundle] pathForResource:@"logo" ofType:@"png"];
        if (iconPath && [[NSFileManager defaultManager] fileExistsAtPath:iconPath]) {
            NSImage *img = [[NSImage alloc] initWithContentsOfFile:iconPath];
            if (img) {
                [img setSize:NSMakeSize(18, 18)];
                [self.statusItem.button setImage:img];
                [self.statusItem.button setTitle:@""];
            } else {
                [self.statusItem.button setTitle:@"📦"];
            }
        } else {
            [self.statusItem.button setTitle:@"📦"];
        }
        [self.statusItem.button setToolTip:[NSString stringWithFormat:@"SlimBox 视频压制服务 (端口: %ld)", (long)self.port]];
    }

    NSMenu *menu = [[NSMenu alloc] init];

    NSMenuItem *openWeb = [[NSMenuItem alloc] initWithTitle:@"🌐 打开 Web 页面" action:@selector(openWeb:) keyEquivalent:@"o"];
    [openWeb setTarget:self];
    [menu addItem:openWeb];

    NSMenuItem *openOutputs = [[NSMenuItem alloc] initWithTitle:@"📁 打开输出目录 (outputs)" action:@selector(openOutputs:) keyEquivalent:@"f"];
    [openOutputs setTarget:self];
    [menu addItem:openOutputs];

    NSMenuItem *chooseDir = [[NSMenuItem alloc] initWithTitle:@"⚙️ 设置存储目录..." action:@selector(chooseDir:) keyEquivalent:@"s"];
    [chooseDir setTarget:self];
    [menu addItem:chooseDir];

    [menu addItem:[NSMenuItem separatorItem]];

    NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"❌ 退出 SlimBox" action:@selector(quitApp:) keyEquivalent:@"q"];
    [quit setTarget:self];
    [menu addItem:quit];

    [self.statusItem setMenu:menu];

    // Automatically open browser after 1 second
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(1.0 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
        [self openWeb:nil];
    });
}

- (void)openWeb:(id)sender {
    NSString *urlStr = [NSString stringWithFormat:@"http://localhost:%ld", (long)self.port];
    [[NSWorkspace sharedWorkspace] openURL:[NSURL URLWithString:urlStr]];
}

- (void)openOutputs:(id)sender {
    NSString *outputs = [self.dataDir stringByAppendingPathComponent:@"outputs"];
    BOOL isDir = NO;
    if ([[NSFileManager defaultManager] fileExistsAtPath:outputs isDirectory:&isDir] && isDir) {
        [[NSWorkspace sharedWorkspace] selectFile:nil inFileViewerRootedAtPath:outputs];
    } else {
        [[NSWorkspace sharedWorkspace] selectFile:nil inFileViewerRootedAtPath:self.dataDir];
    }
}

- (void)chooseDir:(id)sender {
    NSOpenPanel *panel = [NSOpenPanel openPanel];
    [panel setCanChooseFiles:NO];
    [panel setCanChooseDirectories:YES];
    [panel setCanCreateDirectories:YES];
    [panel setPrompt:@"选择"];
    [panel setMessage:@"请选择 SlimBox 视频存储根目录 (将用于保存上传与压缩产物):"];

    if ([panel runModal] == NSModalResponseOK) {
        NSURL *url = [[panel URLs] firstObject];
        if (url) {
            self.dataDir = [url path];
            [self saveConfig];
            
            NSAlert *alert = [[NSAlert alloc] init];
            [alert setMessageText:@"存储目录已更新"];
            [alert setInformativeText:[NSString stringWithFormat:@"SlimBox 存储目录已更改为:\n%@\n\n该设置已保存至 ~/.slimbox/config.json，将在下次启动时生效。", self.dataDir]];
            [alert runModal];
        }
    }
}

- (void)quitApp:(id)sender {
    if (self.serverTask && [self.serverTask isRunning]) {
        [self.serverTask terminate];
    }
    [NSApp terminate:nil];
}

- (void)applicationWillTerminate:(NSNotification *)notification {
    if (self.serverTask && [self.serverTask isRunning]) {
        [self.serverTask terminate];
    }
}

@end

int main(int argc, const char * argv[]) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        AppDelegate *delegate = [[AppDelegate alloc] init];
        [app setDelegate:delegate];
        [app setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [app run];
    }
    return 0;
}
