import 'dart:ui';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'core/theme/app_theme.dart';
import 'core/widgets/mobile_device_frame.dart';
import 'features/main_layout/presentation/main_nav_scaffold.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Edge to edge system overlay
  SystemChrome.setSystemUIOverlayStyle(
    const SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: Brightness.dark,
      statusBarBrightness: Brightness.light,
      systemNavigationBarColor: Colors.white,
      systemNavigationBarIconBrightness: Brightness.dark,
    ),
  );

  runApp(const A2ZUtilsApp());
}

/// Allows smooth click-and-drag gestures on desktop browsers matching touch screens
class MobileScrollBehavior extends MaterialScrollBehavior {
  @override
  Set<PointerDeviceKind> get dragDevices => {
    PointerDeviceKind.touch,
    PointerDeviceKind.mouse,
    PointerDeviceKind.trackpad,
    PointerDeviceKind.stylus,
  };
}

class A2ZUtilsApp extends StatelessWidget {
  const A2ZUtilsApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'a2zutils',
      debugShowCheckedModeBanner: false,
      theme: AppTheme.lightTheme,
      scrollBehavior: MobileScrollBehavior(),
      builder: (context, child) => MobileDeviceFrame(child: child ?? const SizedBox.shrink()),
      home: const MainNavScaffold(),
    );
  }
}
