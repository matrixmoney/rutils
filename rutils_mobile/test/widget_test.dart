import 'package:flutter_test/flutter_test.dart';
import 'package:a2zutils/main.dart';

void main() {
  testWidgets('App smoke test', (WidgetTester tester) async {
    await tester.pumpWidget(const A2ZUtilsApp());
    // Pump and settle flutter_animate animations
    await tester.pumpAndSettle(const Duration(seconds: 1));
    expect(find.byType(A2ZUtilsApp), findsOneWidget);
  });
}
