import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/bouncy_button.dart';
import '../../calculations/presentation/calculations_hub_screen.dart';
import '../../conversion/presentation/conversion_hub_screen.dart';
import '../../home/presentation/home_screen.dart';

class MainNavScaffold extends StatefulWidget {
  const MainNavScaffold({super.key});

  @override
  State<MainNavScaffold> createState() => _MainNavScaffoldState();
}

class _MainNavScaffoldState extends State<MainNavScaffold> {
  int _currentIndex = 0;
  int _conversionInitialTab = 0;
  int _calculationInitialTab = 0;

  void _navigateToTool(int hubIndex, int tabIndex) {
    setState(() {
      _currentIndex = hubIndex;
      if (hubIndex == 1) {
        _conversionInitialTab = tabIndex;
      } else if (hubIndex == 2) {
        _calculationInitialTab = tabIndex;
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    final screens = [
      HomeScreen(onNavigateToTool: _navigateToTool),
      ConversionHubScreen(
        key: ValueKey('conversion_$_conversionInitialTab'),
        initialTabIndex: _conversionInitialTab,
      ),
      CalculationsHubScreen(
        key: ValueKey('calculation_$_calculationInitialTab'),
        initialTabIndex: _calculationInitialTab,
      ),
    ];

    return Scaffold(
      backgroundColor: AppTheme.scaffoldBg,
      body: Stack(
        children: [
          Positioned.fill(
            child: IndexedStack(
              index: _currentIndex,
              children: screens,
            ),
          ),

          // Floating Glassmorphic Bottom Navigation Bar
          Positioned(
            left: 20,
            right: 20,
            bottom: 20,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.92),
                borderRadius: BorderRadius.circular(28),
                border: Border.all(
                  color: Colors.white.withValues(alpha: 0.8),
                  width: 1.5,
                ),
                boxShadow: [
                  BoxShadow(
                    color: AppTheme.primaryDark.withValues(alpha: 0.12),
                    blurRadius: 24,
                    offset: const Offset(0, 8),
                  ),
                ],
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceAround,
                children: [
                  _buildNavItem(
                    index: 0,
                    icon: Icons.grid_view_rounded,
                    label: 'Home',
                  ),
                  _buildNavItem(
                    index: 1,
                    icon: Icons.sync_alt_rounded,
                    label: 'Conversions',
                  ),
                  _buildNavItem(
                    index: 2,
                    icon: Icons.calculate_rounded,
                    label: 'Calculations',
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildNavItem({
    required int index,
    required IconData icon,
    required String label,
  }) {
    final isSelected = _currentIndex == index;

    return BouncyButton(
      onTap: () => setState(() => _currentIndex = index),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 200),
        curve: Curves.easeOutCubic,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        decoration: BoxDecoration(
          color: isSelected ? AppTheme.primary : Colors.transparent,
          borderRadius: BorderRadius.circular(20),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(
              icon,
              size: 20,
              color: isSelected ? Colors.white : AppTheme.textMuted,
            ),
            if (isSelected) ...[
              const SizedBox(width: 8),
              Text(
                label,
                style: GoogleFonts.plusJakartaSans(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  color: Colors.white,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
