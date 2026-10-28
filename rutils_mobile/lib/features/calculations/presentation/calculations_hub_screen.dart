import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/bouncy_button.dart';
import 'age_calculator_tab.dart';
import 'bmi_calculator_tab.dart';
import 'emi_calculator_tab.dart';
import 'percentage_calculator_tab.dart';
import 'tax_calculator_tab.dart';

class CalculationsHubScreen extends StatefulWidget {
  final int initialTabIndex;
  const CalculationsHubScreen({super.key, this.initialTabIndex = 0});

  @override
  State<CalculationsHubScreen> createState() => _CalculationsHubScreenState();
}

class _CalculationsHubScreenState extends State<CalculationsHubScreen> {
  late int _selectedTab;
  late final PageController _pageController;

  @override
  void initState() {
    super.initState();
    _selectedTab = widget.initialTabIndex.clamp(0, 4);
    _pageController = PageController(initialPage: _selectedTab);
  }

  final List<({String title, IconData icon, Widget screen})> _tabs = [
    (title: 'BMI', icon: Icons.monitor_weight_rounded, screen: const BmiCalculatorTab()),
    (title: 'Age', icon: Icons.cake_rounded, screen: const AgeCalculatorTab()),
    (title: 'Percentage', icon: Icons.percent_rounded, screen: const PercentageCalculatorTab()),
    (title: 'Loan EMI', icon: Icons.account_balance_rounded, screen: const EmiCalculatorTab()),
    (title: 'Tax / GST', icon: Icons.receipt_long_rounded, screen: const TaxCalculatorTab()),
  ];

  @override
  void dispose() {
    _pageController.dispose();
    super.dispose();
  }

  void _onTabTapped(int index) {
    setState(() => _selectedTab = index);
    _pageController.animateToPage(
      index,
      duration: const Duration(milliseconds: 250),
      curve: Curves.easeInOut,
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppTheme.scaffoldBg,
      body: SafeArea(
        child: Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
              child: Row(
                children: [
                  Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(
                      gradient: AppTheme.skyAccentGradient,
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: const Icon(Icons.calculate_rounded, color: Colors.white, size: 20),
                  ),
                  const SizedBox(width: 12),
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Financial & Everyday Math',
                        style: GoogleFonts.plusJakartaSans(
                          fontSize: 18,
                          fontWeight: FontWeight.w800,
                          color: AppTheme.textPrimary,
                        ),
                      ),
                      Text(
                        'EMI, Tax, Percentage, Age & BMI calculators',
                        style: GoogleFonts.plusJakartaSans(
                          fontSize: 12,
                          color: AppTheme.textMuted,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            Container(
              height: 44,
              margin: const EdgeInsets.only(bottom: 6),
              child: ListView.builder(
                scrollDirection: Axis.horizontal,
                physics: const BouncingScrollPhysics(),
                padding: const EdgeInsets.symmetric(horizontal: 16),
                itemCount: _tabs.length,
                itemBuilder: (context, index) {
                  final tab = _tabs[index];
                  final isSelected = _selectedTab == index;

                  return Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: BouncyButton(
                      onTap: () => _onTabTapped(index),
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 14),
                        decoration: BoxDecoration(
                          color: isSelected ? AppTheme.primary : Colors.white,
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(
                            color: isSelected ? AppTheme.primary : const Color(0xFFE2E8F0),
                          ),
                          boxShadow: isSelected ? AppTheme.softShadow : null,
                        ),
                        child: Row(
                          children: [
                            Icon(tab.icon, size: 16, color: isSelected ? Colors.white : AppTheme.textSecondary),
                            const SizedBox(width: 6),
                            Text(
                              tab.title,
                              style: GoogleFonts.plusJakartaSans(
                                fontSize: 13,
                                fontWeight: FontWeight.w600,
                                color: isSelected ? Colors.white : AppTheme.textSecondary,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  );
                },
              ),
            ),
            Expanded(
              child: PageView(
                controller: _pageController,
                onPageChanged: (idx) => setState(() => _selectedTab = idx),
                children: _tabs.map((t) => t.screen).toList(),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
