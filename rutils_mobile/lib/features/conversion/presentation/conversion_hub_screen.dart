import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/bouncy_button.dart';
import 'file_converter_tab.dart';
import 'measurement_tab.dart';
import 'numeral_tab.dart';
import 'pdf_compressor_tab.dart';
import 'railway_tab.dart';
import 'timezone_tab.dart';

class ConversionHubScreen extends StatefulWidget {
  final int initialTabIndex;
  const ConversionHubScreen({super.key, this.initialTabIndex = 0});

  @override
  State<ConversionHubScreen> createState() => _ConversionHubScreenState();
}

class _ConversionHubScreenState extends State<ConversionHubScreen> {
  late int _selectedTab;
  late final PageController _pageController;

  @override
  void initState() {
    super.initState();
    _selectedTab = widget.initialTabIndex.clamp(0, 5);
    _pageController = PageController(initialPage: _selectedTab);
  }

  final List<({String title, IconData icon, Widget screen})> _tabs = [
    (title: 'File', icon: Icons.transform_rounded, screen: const FileConverterTab()),
    (title: 'PDF Size', icon: Icons.picture_as_pdf_rounded, screen: const PdfCompressorTab()),
    (title: 'Units', icon: Icons.straighten_rounded, screen: const MeasurementTab()),
    (title: 'Time Zones', icon: Icons.access_time_rounded, screen: const TimezoneTab()),
    (title: 'Railway', icon: Icons.train_rounded, screen: const RailwayTab()),
    (title: 'Numeral', icon: Icons.tag_rounded, screen: const NumeralTab()),
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
            // Top App Bar
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
              child: Row(
                children: [
                  Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(
                      gradient: AppTheme.primaryGradient,
                      borderRadius: BorderRadius.circular(12),
                    ),
                    child: const Icon(Icons.sync_alt_rounded, color: Colors.white, size: 20),
                  ),
                  const SizedBox(width: 12),
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Conversion Suite',
                        style: GoogleFonts.plusJakartaSans(
                          fontSize: 18,
                          fontWeight: FontWeight.w800,
                          color: AppTheme.textPrimary,
                        ),
                      ),
                      Text(
                        'Transform files, units & timestamps',
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

            // Horizontal Tab Bar
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
                            Icon(
                              tab.icon,
                              size: 16,
                              color: isSelected ? Colors.white : AppTheme.textSecondary,
                            ),
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

            // Page View
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
