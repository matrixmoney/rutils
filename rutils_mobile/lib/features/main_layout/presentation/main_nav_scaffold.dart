import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/bouncy_button.dart';
import '../../calculations/presentation/calculations_hub_screen.dart';
import '../../conversion/presentation/conversion_hub_screen.dart';
import '../../profile/presentation/profile_screen.dart';

class MainNavScaffold extends StatefulWidget {
  const MainNavScaffold({super.key});

  @override
  State<MainNavScaffold> createState() => _MainNavScaffoldState();
}

class _MainNavScaffoldState extends State<MainNavScaffold> with SingleTickerProviderStateMixin {
  int _currentIndex = 0;
  bool _isNavOpen = false;

  final List<Widget> _screens = const [
    ConversionHubScreen(),
    CalculationsHubScreen(),
    ProfileScreen(),
  ];

  final List<({String label, String subtitle, IconData icon, IconData activeIcon})> _navItems = const [
    (
      label: 'Conversions',
      subtitle: 'File, PDF, Units, Time & Numeral',
      icon: Icons.sync_alt_outlined,
      activeIcon: Icons.sync_alt_rounded,
    ),
    (
      label: 'Calculations',
      subtitle: 'EMI, BMI, Tax/GST, Age & Percentage',
      icon: Icons.calculate_outlined,
      activeIcon: Icons.calculate_rounded,
    ),
    (
      label: 'Profile',
      subtitle: 'Account, Security, Latency & Logout',
      icon: Icons.person_outline_rounded,
      activeIcon: Icons.person_rounded,
    ),
  ];

  void _selectTab(int index) {
    setState(() {
      _currentIndex = index;
      _isNavOpen = false;
    });
  }

  @override
  Widget build(BuildContext context) {
    final activeItem = _navItems[_currentIndex];

    return Scaffold(
      backgroundColor: AppTheme.scaffoldBg,
      body: Stack(
        children: [
          // 1. Full-Height Active Tool Screen
          Positioned.fill(
            child: IndexedStack(
              index: _currentIndex,
              children: _screens,
            ),
          ),

          // 2. Translucent Backdrop Barrier (shown only when navigation is opened)
          if (_isNavOpen)
            Positioned.fill(
              child: GestureDetector(
                onTap: () => setState(() => _isNavOpen = false),
                child: Container(
                  color: Colors.black.withValues(alpha: 0.45),
                ),
              ),
            ),

          // 3. Clickable Navigation Bar / Drawer (Revealed ONLY when clicked)
          AnimatedPositioned(
            duration: const Duration(milliseconds: 250),
            curve: Curves.easeOutCubic,
            left: 16,
            right: 16,
            bottom: _isNavOpen ? 24 : -450,
            child: AnimatedOpacity(
              duration: const Duration(milliseconds: 220),
              opacity: _isNavOpen ? 1.0 : 0.0,
              child: Container(
                padding: const EdgeInsets.fromLTRB(20, 16, 20, 20),
                decoration: BoxDecoration(
                  color: Colors.white,
                  borderRadius: BorderRadius.circular(28),
                  border: Border.all(color: const Color(0xFFE2E8F0), width: 1.2),
                  boxShadow: [
                    BoxShadow(
                      color: const Color(0xFF0F172A).withValues(alpha: 0.25),
                      blurRadius: 36,
                      spreadRadius: 2,
                      offset: const Offset(0, 12),
                    ),
                  ],
                ),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    // Header with title and close button
                    Row(
                      mainAxisAlignment: MainAxisAlignment.spaceBetween,
                      children: [
                        Row(
                          children: [
                            Container(
                              padding: const EdgeInsets.all(7),
                              decoration: BoxDecoration(
                                color: AppTheme.primary.withValues(alpha: 0.1),
                                borderRadius: BorderRadius.circular(10),
                              ),
                              child: const Icon(
                                Icons.explore_rounded,
                                color: AppTheme.primary,
                                size: 18,
                              ),
                            ),
                            const SizedBox(width: 10),
                            Text(
                              'Navigation Menu',
                              style: GoogleFonts.plusJakartaSans(
                                fontSize: 16,
                                fontWeight: FontWeight.w700,
                                color: AppTheme.textPrimary,
                              ),
                            ),
                          ],
                        ),
                        IconButton(
                          onPressed: () => setState(() => _isNavOpen = false),
                          icon: const Icon(Icons.close_rounded, size: 20),
                          style: IconButton.styleFrom(
                            backgroundColor: const Color(0xFFF1F5F9),
                            foregroundColor: AppTheme.textSecondary,
                            padding: const EdgeInsets.all(6),
                            minimumSize: const Size(32, 32),
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 14),

                    // Navigation Items List
                    ...List.generate(_navItems.length, (index) {
                      final item = _navItems[index];
                      final isSelected = _currentIndex == index;

                      return Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: BouncyButton(
                          onTap: () => _selectTab(index),
                          child: AnimatedContainer(
                            duration: const Duration(milliseconds: 180),
                            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
                            decoration: BoxDecoration(
                              gradient: isSelected
                                  ? const LinearGradient(
                                      colors: [Color(0xFF1E40AF), Color(0xFF2563EB)],
                                      begin: Alignment.topLeft,
                                      end: Alignment.bottomRight,
                                    )
                                  : null,
                              color: isSelected ? null : const Color(0xFFF8FAFC),
                              borderRadius: BorderRadius.circular(16),
                              border: Border.all(
                                color: isSelected
                                    ? Colors.transparent
                                    : const Color(0xFFE2E8F0),
                              ),
                            ),
                            child: Row(
                              children: [
                                Container(
                                  padding: const EdgeInsets.all(8),
                                  decoration: BoxDecoration(
                                    color: isSelected
                                        ? Colors.white.withValues(alpha: 0.2)
                                        : AppTheme.primary.withValues(alpha: 0.08),
                                    borderRadius: BorderRadius.circular(12),
                                  ),
                                  child: Icon(
                                    isSelected ? item.activeIcon : item.icon,
                                    color: isSelected ? Colors.white : AppTheme.primary,
                                    size: 20,
                                  ),
                                ),
                                const SizedBox(width: 14),
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment: CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        item.label,
                                        style: GoogleFonts.plusJakartaSans(
                                          fontSize: 14,
                                          fontWeight: FontWeight.w700,
                                          color: isSelected ? Colors.white : AppTheme.textPrimary,
                                        ),
                                      ),
                                      const SizedBox(height: 2),
                                      Text(
                                        item.subtitle,
                                        style: GoogleFonts.plusJakartaSans(
                                          fontSize: 11,
                                          color: isSelected
                                              ? const Color(0xFFBFDBFE)
                                              : AppTheme.textMuted,
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                                if (isSelected)
                                  const Icon(
                                    Icons.check_circle_rounded,
                                    color: Colors.white,
                                    size: 20,
                                  ),
                              ],
                            ),
                          ),
                        ),
                      );
                    }),
                  ],
                ),
              ),
            ),
          ),

          // 4. Sleek Floating Trigger Button (Always visible at bottom center when menu is closed)
          if (!_isNavOpen)
            Positioned(
              left: 0,
              right: 0,
              bottom: 20,
              child: Center(
                child: BouncyButton(
                  onTap: () => setState(() => _isNavOpen = true),
                  child: Container(
                    padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 10),
                    decoration: BoxDecoration(
                      gradient: const LinearGradient(
                        colors: [Color(0xFF1E3A8A), Color(0xFF2563EB)],
                        begin: Alignment.topLeft,
                        end: Alignment.bottomRight,
                      ),
                      borderRadius: BorderRadius.circular(30),
                      boxShadow: [
                        BoxShadow(
                          color: const Color(0xFF1E40AF).withValues(alpha: 0.4),
                          blurRadius: 20,
                          spreadRadius: 1,
                          offset: const Offset(0, 6),
                        ),
                      ],
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(
                          activeItem.activeIcon,
                          color: Colors.white,
                          size: 18,
                        ),
                        const SizedBox(width: 8),
                        Text(
                          activeItem.label,
                          style: GoogleFonts.plusJakartaSans(
                            color: Colors.white,
                            fontSize: 13,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(width: 8),
                        Container(
                          padding: const EdgeInsets.all(3),
                          decoration: BoxDecoration(
                            color: Colors.white.withValues(alpha: 0.2),
                            shape: BoxShape.circle,
                          ),
                          child: const Icon(
                            Icons.keyboard_arrow_up_rounded,
                            color: Colors.white,
                            size: 16,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
