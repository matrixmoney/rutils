import 'package:flutter/foundation.dart';

class ApiConstants {
  // Configurable at compile time via --dart-define=API_URL=https://...
  static const String _envApiUrl = String.fromEnvironment('API_URL', defaultValue: '');

  // Default endpoints
  static const String prodBaseUrl = 'https://utils.api.srilakshmiretail.in';
  static const String localWebDesktopBaseUrl = 'http://localhost:8080';
  static const String localAndroidEmulatorBaseUrl = 'http://10.0.2.2:8080';

  /// Resolves the default base URL based on build mode and platform:
  /// - Web preview strictly connects to local backend on http://localhost:8080
  /// - Native mobile (Android APK, iOS) strictly connects to real production backend https://utils.api.srilakshmiretail.in
  static String getDefaultBaseUrl() {
    if (_envApiUrl.isNotEmpty) {
      return _envApiUrl;
    }

    // Local web preview runs on localhost:8087 and communicates with local Go backend on localhost:8080
    if (kIsWeb) {
      return localWebDesktopBaseUrl;
    }

    // Android APK / iOS strictly connects to real production backend
    return prodBaseUrl;
  }

  // Conversion Endpoints
  static const String convertFile = '/convert';
  static const String convertPdfSize = '/convert-pdf-size';
  static const String convertMeasurement = '/convert-measurement';
  static const String convertTime = '/convert-time';
  static const String convertRailway = '/convert-railway';
  static const String convertNumeral = '/convert-numeral';

  // Calculations Endpoints
  static const String calculateBmi = '/calculate-bmi';
  static const String calculateAge = '/calculate-age';
  static const String calculatePercentage = '/calculate-percentage';
  static const String calculateEmi = '/calculate-emi';
  static const String calculateTax = '/calculate-tax';
}
