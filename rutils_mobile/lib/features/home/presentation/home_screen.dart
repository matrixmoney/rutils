import 'package:flutter/material.dart';
import 'package:flutter_animate/flutter_animate.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../core/theme/app_theme.dart';
import '../../../core/widgets/bouncy_button.dart';

class HomeScreen extends StatefulWidget {
  final Function(int hubIndex, int tabIndex) onNavigateToTool;

  const HomeScreen({
    super.key,
    required this.onNavigateToTool,
  });

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  String _searchQuery = '';
  String _selectedCategory = 'All';

  final List<({
    String title,
    String subtitle,
    IconData icon,
    Color startColor,
    Color endColor,
    int hubIndex, // 1: Conversions, 2: Calculations
    int tabIndex,
    String category,
    String badge,
  })> _allTools = const [
    // Conversion Tools
    (
      title: 'File Converter',
      subtitle: 'Images, PDFs & media formats',
      icon: Icons.transform_rounded,
      startColor: Color(0xFF2563EB),
      endColor: Color(0xFF0EA5E9),
      hubIndex: 1,
      tabIndex: 0,
      category: 'Conversion',
      badge: 'Cloud Sync',
    ),
    (
      title: 'PDF Compressor',
      subtitle: 'Reduce file size with smart compression',
      icon: Icons.picture_as_pdf_rounded,
      startColor: Color(0xFFE11D48),
      endColor: Color(0xFFF97316),
      hubIndex: 1,
      tabIndex: 1,
      category: 'Conversion',
      badge: 'Optimized',
    ),
    (
      title: 'Unit Measurements',
      subtitle: 'Length, mass, area, temp & volume',
      icon: Icons.straighten_rounded,
      startColor: Color(0xFF0D9488),
      endColor: Color(0xFF10B981),
      hubIndex: 1,
      tabIndex: 2,
      category: 'Conversion',
      badge: 'Instant',
    ),
    (
      title: 'Time Zones',
      subtitle: 'Global real-time world clock offsets',
      icon: Icons.access_time_rounded,
      startColor: Color(0xFF4F46E5),
      endColor: Color(0xFF818CF8),
      hubIndex: 1,
      tabIndex: 3,
      category: 'Conversion',
      badge: 'Live UTC',
    ),
    (
      title: 'Railway Time',
      subtitle: '24-hour military & 12-hour converter',
      icon: Icons.train_rounded,
      startColor: Color(0xFF0284C7),
      endColor: Color(0xFF38BDF8),
      hubIndex: 1,
      tabIndex: 4,
      category: 'Conversion',
      badge: '24-Hr',
    ),
    (
      title: 'Numeral System',
      subtitle: 'Binary, Hex, Octal & Roman numbers',
      icon: Icons.tag_rounded,
      startColor: Color(0xFF7C3AED),
      endColor: Color(0xFFA855F7),
      hubIndex: 1,
      tabIndex: 5,
      category: 'Conversion',
      badge: 'Radix',
    ),

    // Calculation Tools
    (
      title: 'Loan EMI Calculator',
      subtitle: 'Monthly installments, interest & schedules',
      icon: Icons.account_balance_rounded,
      startColor: Color(0xFF16A34A),
      endColor: Color(0xFF22C55E),
      hubIndex: 2,
      tabIndex: 3,
      category: 'Calculation',
      badge: 'Finance',
    ),
    (
      title: 'Tax & GST Calculator',
      subtitle: 'Instant GST slabs & reverse taxation',
      icon: Icons.receipt_long_rounded,
      startColor: Color(0xFFD97706),
      endColor: Color(0xFFF59E0B),
      hubIndex: 2,
      tabIndex: 4,
      category: 'Calculation',
      badge: 'GST/Tax',
    ),
    (
      title: 'BMI Calculator',
      subtitle: 'Body mass index & health analytics',
      icon: Icons.monitor_weight_rounded,
      startColor: Color(0xFFBE185D),
      endColor: Color(0xFFFB7185),
      hubIndex: 2,
      tabIndex: 0,
      category: 'Calculation',
      badge: 'Health',
    ),
    (
      title: 'Age & Milestone',
      subtitle: 'Exact age, months, leap days & countdown',
      icon: Icons.cake_rounded,
      startColor: Color(0xFF9333EA),
      endColor: Color(0xFFC084FC),
      hubIndex: 2,
      tabIndex: 1,
      category: 'Calculation',
      badge: 'Exact',
    ),
    (
      title: 'Percentage Calculator',
      subtitle: 'Discounts, growth percentages & ratio',
      icon: Icons.percent_rounded,
      startColor: Color(0xFF0284C7),
      endColor: Color(0xFF06B6D4),
      hubIndex: 2,
      tabIndex: 2,
      category: 'Calculation',
      badge: 'Math',
    ),
  ];

  @override
  Widget build(BuildContext context) {
    final filteredTools = _allTools.where((t) {
      final matchesSearch = _searchQuery.isEmpty ||
          t.title.toLowerCase().contains(_searchQuery.toLowerCase()) ||
          t.subtitle.toLowerCase().contains(_searchQuery.toLowerCase()) ||
          t.category.toLowerCase().contains(_searchQuery.toLowerCase());
      final matchesCat = _selectedCategory == 'All' || t.category == _selectedCategory;
      return matchesSearch && matchesCat;
    }).toList();

    return Scaffold(
      backgroundColor: const Color(0xFFF8FAFC),
      body: SafeArea(
        child: CustomScrollView(
          physics: const BouncingScrollPhysics(),
          slivers: [
            // Creative Futuristic Top Bar & Hero Section
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.fromLTRB(20, 14, 20, 16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Brand Bar with Live Status indicator
                    _buildTopStatusBar(),
                    const SizedBox(height: 16),

                    // Creative Dynamic Banner with Clean Emblem & Glowing Gradient Accents
                    _buildUltraCreativeBanner(),
                    const SizedBox(height: 18),

                    // Live Cloud Pulse Info Card
                    _buildCloudLiveBar(),
                    const SizedBox(height: 18),

                    // Sleek Modern Search Bar
                    _buildSearchBar(),
                    const SizedBox(height: 16),

                    // Category Filter Pills
                    _buildCategoryFilterPills(),
                  ],
                ),
              ),
            ),

            // Tools Direct Grid Navigation
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 4, 20, 100),
              sliver: SliverGrid(
                gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: 2,
                  mainAxisSpacing: 16,
                  crossAxisSpacing: 16,
                  childAspectRatio: 0.88,
                ),
                delegate: SliverChildBuilderDelegate(
                  (context, index) {
                    final tool = filteredTools[index];
                    return _buildToolCard(tool, index);
                  },
                  childCount: filteredTools.length,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildTopStatusBar() {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Row(
          children: [
            Container(
              width: 10,
              height: 10,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: const Color(0xFF10B981),
                boxShadow: [
                  BoxShadow(
                    color: const Color(0xFF10B981).withValues(alpha: 0.6),
                    blurRadius: 8,
                    spreadRadius: 2,
                  ),
                ],
              ),
            ),
            const SizedBox(width: 8),
            Text(
              'CLOUD SERVICES ACTIVE',
              style: GoogleFonts.plusJakartaSans(
                fontSize: 11,
                fontWeight: FontWeight.w800,
                color: const Color(0xFF059669),
                letterSpacing: 0.8,
              ),
            ),
          ],
        ),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
          decoration: BoxDecoration(
            color: Colors.white,
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: const Color(0xFFE2E8F0)),
            boxShadow: [
              BoxShadow(
                color: Colors.black.withValues(alpha: 0.03),
                blurRadius: 6,
                offset: const Offset(0, 2),
              ),
            ],
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.cloud_done_rounded, size: 14, color: Color(0xFF2563EB)),
              const SizedBox(width: 5),
              Text(
                'v1.0.0 Online',
                style: GoogleFonts.plusJakartaSans(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  color: AppTheme.textPrimary,
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildUltraCreativeBanner() {
    return Container(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(30),
        gradient: const LinearGradient(
          colors: [
            Color(0xFF0B132B), // Deep galactic navy
            Color(0xFF1C2541), // Rich slate
            Color(0xFF1E3A8A), // Royal blue depth
          ],
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
        ),
        boxShadow: [
          BoxShadow(
            color: const Color(0xFF1E3A8A).withValues(alpha: 0.38),
            blurRadius: 32,
            offset: const Offset(0, 16),
            spreadRadius: -4,
          ),
        ],
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(30),
        child: Stack(
          children: [
            // Ambient neon light orb 1 (Cyan)
            Positioned(
              right: -30,
              top: -30,
              child: Container(
                width: 190,
                height: 190,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: const Color(0xFF00F0FF).withValues(alpha: 0.22),
                ),
              ),
            ),

            // Ambient neon light orb 2 (Deep Violet)
            Positioned(
              left: -20,
              bottom: -40,
              child: Container(
                width: 170,
                height: 170,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: const Color(0xFF8B5CF6).withValues(alpha: 0.20),
                ),
              ),
            ),

            // Diagonal glossy glass accent stripe
            Positioned(
              top: -60,
              right: 60,
              width: 120,
              height: 280,
              child: Transform.rotate(
                angle: 0.45,
                child: Container(
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      colors: [
                        Colors.white.withValues(alpha: 0.08),
                        Colors.white.withValues(alpha: 0.0),
                      ],
                      begin: Alignment.topCenter,
                      end: Alignment.bottomCenter,
                    ),
                  ),
                ),
              ),
            ),

            // Foreground Banner Content
            Padding(
              padding: const EdgeInsets.all(22),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    crossAxisAlignment: CrossAxisAlignment.center,
                    children: [
                      // Perfectly Fitted Logo Card with 3D Depth & Glass Border
                      Container(
                        width: 76,
                        height: 76,
                        padding: const EdgeInsets.all(10),
                        decoration: BoxDecoration(
                          borderRadius: BorderRadius.circular(22),
                          gradient: LinearGradient(
                            colors: [
                              const Color(0xFF1E293B).withValues(alpha: 0.9),
                              const Color(0xFF0F172A).withValues(alpha: 0.95),
                            ],
                            begin: Alignment.topLeft,
                            end: Alignment.bottomRight,
                          ),
                          border: Border.all(
                            color: Colors.white.withValues(alpha: 0.22),
                            width: 1.5,
                          ),
                          boxShadow: [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.45),
                              blurRadius: 18,
                              offset: const Offset(0, 8),
                            ),
                            BoxShadow(
                              color: const Color(0xFF00F0FF).withValues(alpha: 0.35),
                              blurRadius: 16,
                              spreadRadius: -2,
                            ),
                          ],
                        ),
                        child: Image.asset(
                          'assets/images/emblem_clean.png',
                          fit: BoxFit.contain,
                        ),
                      ).animate().scale(
                            duration: 500.ms,
                            curve: Curves.easeOutBack,
                          ),
                      const SizedBox(width: 16),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Row(
                              children: [
                                Text(
                                  'a2zutils',
                                  style: GoogleFonts.plusJakartaSans(
                                    fontSize: 27,
                                    fontWeight: FontWeight.w900,
                                    color: Colors.white,
                                    letterSpacing: -0.6,
                                  ),
                                ),
                                const SizedBox(width: 8),
                                Container(
                                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                                  decoration: BoxDecoration(
                                    gradient: const LinearGradient(
                                      colors: [Color(0xFF00F0FF), Color(0xFF3B82F6)],
                                    ),
                                    borderRadius: BorderRadius.circular(10),
                                    boxShadow: [
                                      BoxShadow(
                                        color: const Color(0xFF00F0FF).withValues(alpha: 0.4),
                                        blurRadius: 8,
                                      ),
                                    ],
                                  ),
                                  child: Text(
                                    'PRO',
                                    style: GoogleFonts.plusJakartaSans(
                                      fontSize: 10,
                                      fontWeight: FontWeight.w900,
                                      color: const Color(0xFF0B132B),
                                      letterSpacing: 0.6,
                                    ),
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 4),
                            Text(
                              'The Ultimate Online Power Suite',
                              style: GoogleFonts.plusJakartaSans(
                                fontSize: 13,
                                fontWeight: FontWeight.w600,
                                color: const Color(0xFF38BDF8),
                              ),
                            ),
                            const SizedBox(height: 2),
                            Text(
                              'High-speed cloud converters & precision engines',
                              style: GoogleFonts.plusJakartaSans(
                                fontSize: 11,
                                fontWeight: FontWeight.w500,
                                color: Colors.white.withValues(alpha: 0.75),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 18),

                  // Interactive Feature Metric Highlights
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
                    decoration: BoxDecoration(
                      color: Colors.white.withValues(alpha: 0.08),
                      borderRadius: BorderRadius.circular(18),
                      border: Border.all(
                        color: Colors.white.withValues(alpha: 0.14),
                      ),
                    ),
                    child: Row(
                      mainAxisAlignment: MainAxisAlignment.spaceAround,
                      children: [
                        _buildBannerMetric(
                          icon: Icons.flash_on_rounded,
                          title: '0.12s',
                          subtitle: 'Cloud Speed',
                          color: const Color(0xFF00F0FF),
                        ),
                        Container(
                          width: 1,
                          height: 28,
                          color: Colors.white.withValues(alpha: 0.15),
                        ),
                        _buildBannerMetric(
                          icon: Icons.architecture_rounded,
                          title: '11 Tools',
                          subtitle: 'Precision Suite',
                          color: const Color(0xFFF59E0B),
                        ),
                        Container(
                          width: 1,
                          height: 28,
                          color: Colors.white.withValues(alpha: 0.15),
                        ),
                        _buildBannerMetric(
                          icon: Icons.shield_rounded,
                          title: '100% Safe',
                          subtitle: 'Encrypted Cloud',
                          color: const Color(0xFF10B981),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildBannerMetric({
    required IconData icon,
    required String title,
    required String subtitle,
    required Color color,
  }) {
    return Row(
      children: [
        Icon(icon, color: color, size: 20),
        const SizedBox(width: 8),
        Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              title,
              style: GoogleFonts.plusJakartaSans(
                fontSize: 12,
                fontWeight: FontWeight.w800,
                color: Colors.white,
              ),
            ),
            Text(
              subtitle,
              style: GoogleFonts.plusJakartaSans(
                fontSize: 10,
                fontWeight: FontWeight.w500,
                color: Colors.white.withValues(alpha: 0.7),
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildCloudLiveBar() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: const Color(0xFFE2E8F0)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.02),
            blurRadius: 10,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: const Color(0xFFEEF2FF),
              borderRadius: BorderRadius.circular(12),
            ),
            child: const Icon(
              Icons.wifi_tethering_rounded,
              color: Color(0xFF4F46E5),
              size: 20,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Connected to a2z Server Engine',
                  style: GoogleFonts.plusJakartaSans(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: AppTheme.textPrimary,
                  ),
                ),
                Text(
                  'Cloud-backed conversions with zero device load',
                  style: GoogleFonts.plusJakartaSans(
                    fontSize: 11,
                    fontWeight: FontWeight.w500,
                    color: AppTheme.textSecondary,
                  ),
                ),
              ],
            ),
          ),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: const Color(0xFFDCFCE7),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Text(
              'ONLINE',
              style: GoogleFonts.plusJakartaSans(
                fontSize: 10,
                fontWeight: FontWeight.w800,
                color: const Color(0xFF15803D),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildSearchBar() {
    return Container(
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: const Color(0xFFE2E8F0)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.03),
            blurRadius: 10,
            offset: const Offset(0, 4),
          ),
        ],
      ),
      child: TextField(
        onChanged: (val) => setState(() => _searchQuery = val),
        style: GoogleFonts.plusJakartaSans(
          fontSize: 14,
          fontWeight: FontWeight.w600,
          color: AppTheme.textPrimary,
        ),
        decoration: InputDecoration(
          hintText: 'Search online tools, converters or calculators...',
          hintStyle: GoogleFonts.plusJakartaSans(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: AppTheme.textMuted,
          ),
          prefixIcon: const Icon(
            Icons.search_rounded,
            color: Color(0xFF2563EB),
            size: 22,
          ),
          suffixIcon: _searchQuery.isNotEmpty
              ? IconButton(
                  icon: const Icon(Icons.clear_rounded, size: 18, color: AppTheme.textMuted),
                  onPressed: () => setState(() => _searchQuery = ''),
                )
              : null,
          border: InputBorder.none,
          contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        ),
      ),
    );
  }

  Widget _buildCategoryFilterPills() {
    final categories = ['All', 'Conversion', 'Calculation'];

    return Row(
      children: categories.map((cat) {
        final isSelected = _selectedCategory == cat;
        return Padding(
          padding: const EdgeInsets.only(right: 8),
          child: BouncyButton(
            onTap: () => setState(() => _selectedCategory = cat),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 200),
              curve: Curves.easeOutCubic,
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
              decoration: BoxDecoration(
                color: isSelected ? const Color(0xFF0F172A) : Colors.white,
                borderRadius: BorderRadius.circular(20),
                border: Border.all(
                  color: isSelected ? const Color(0xFF0F172A) : const Color(0xFFE2E8F0),
                ),
                boxShadow: isSelected
                    ? [
                        BoxShadow(
                          color: Colors.black.withValues(alpha: 0.15),
                          blurRadius: 8,
                          offset: const Offset(0, 4),
                        ),
                      ]
                    : [],
              ),
              child: Text(
                cat == 'All' ? '⚡ All Utilities' : (cat == 'Conversion' ? '🔄 Conversions' : '🧮 Calculations'),
                style: GoogleFonts.plusJakartaSans(
                  fontSize: 12,
                  fontWeight: isSelected ? FontWeight.w800 : FontWeight.w600,
                  color: isSelected ? Colors.white : AppTheme.textSecondary,
                ),
              ),
            ),
          ),
        );
      }).toList(),
    );
  }

  Widget _buildToolCard(
    ({
      String title,
      String subtitle,
      IconData icon,
      Color startColor,
      Color endColor,
      int hubIndex,
      int tabIndex,
      String category,
      String badge,
    }) tool,
    int index,
  ) {
    return BouncyButton(
      onTap: () => widget.onNavigateToTool(tool.hubIndex, tool.tabIndex),
      child: Container(
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(24),
          border: Border.all(
            color: const Color(0xFFEEF2F6),
            width: 1.2,
          ),
          boxShadow: [
            BoxShadow(
              color: tool.startColor.withValues(alpha: 0.09),
              blurRadius: 18,
              offset: const Offset(0, 8),
              spreadRadius: -2,
            ),
          ],
        ),
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                // 3D Glass Icon Container
                Container(
                  width: 46,
                  height: 46,
                  decoration: BoxDecoration(
                    borderRadius: BorderRadius.circular(16),
                    gradient: LinearGradient(
                      colors: [tool.startColor, tool.endColor],
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                    ),
                    boxShadow: [
                      BoxShadow(
                        color: tool.startColor.withValues(alpha: 0.4),
                        blurRadius: 12,
                        offset: const Offset(0, 6),
                      ),
                    ],
                  ),
                  child: Icon(
                    tool.icon,
                    color: Colors.white,
                    size: 24,
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  decoration: BoxDecoration(
                    color: tool.startColor.withValues(alpha: 0.09),
                    borderRadius: BorderRadius.circular(10),
                  ),
                  child: Text(
                    tool.badge,
                    style: GoogleFonts.plusJakartaSans(
                      fontSize: 10,
                      fontWeight: FontWeight.w800,
                      color: tool.startColor,
                    ),
                  ),
                ),
              ],
            ),
            const Spacer(),
            Text(
              tool.title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: GoogleFonts.plusJakartaSans(
                fontSize: 15,
                fontWeight: FontWeight.w800,
                color: AppTheme.textPrimary,
                letterSpacing: -0.2,
              ),
            ),
            const SizedBox(height: 4),
            Text(
              tool.subtitle,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
              style: GoogleFonts.plusJakartaSans(
                fontSize: 11,
                fontWeight: FontWeight.w500,
                color: AppTheme.textSecondary,
                height: 1.3,
              ),
            ),
            const SizedBox(height: 10),
            Row(
              children: [
                Text(
                  'Launch Tool',
                  style: GoogleFonts.plusJakartaSans(
                    fontSize: 12,
                    fontWeight: FontWeight.w700,
                    color: tool.startColor,
                  ),
                ),
                const SizedBox(width: 4),
                Icon(
                  Icons.arrow_forward_rounded,
                  size: 13,
                  color: tool.startColor,
                ),
              ],
            ),
          ],
        ),
      ),
    ).animate().fadeIn(duration: 350.ms, delay: (index * 35).ms).slideY(begin: 0.08, end: 0);
  }
}
