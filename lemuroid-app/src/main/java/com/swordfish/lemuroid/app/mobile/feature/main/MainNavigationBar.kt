package com.swordfish.lemuroid.app.mobile.feature.main

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.Icon
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.navigation.NavHostController
import com.swordfish.lemuroid.app.mobile.shared.compose.ui.PokedexColors

@Composable
fun MainNavigationBar(
    currentRoute: MainRoute?,
    navController: NavHostController,
) {
    AnimatedVisibility(
        visible = currentRoute?.showBottomNavigation != false,
        enter = expandVertically(),
        exit = shrinkVertically(),
    ) {
        LemuroidNavigationBar(currentRoute, navController)
    }
}

@Composable
private fun LemuroidNavigationBar(
    currentRoute: MainRoute?,
    navController: NavHostController,
) {
    NavigationBar(
        modifier = Modifier.fillMaxWidth(),
        containerColor = PokedexColors.Frame,
        contentColor = PokedexColors.Screen,
    ) {
        MainNavigationRoutes.values().forEach { destination ->
            val isSelected = currentRoute?.root == destination.route
            val iconDrawable = if (isSelected) destination.selectedIcon else destination.unselectedIcon

            NavigationBarItem(
                icon = {
                    Icon(
                        imageVector = iconDrawable,
                        contentDescription = stringResource(destination.titleId),
                    )
                },
                label = { Text(stringResource(destination.titleId)) },
                selected = isSelected,
                onClick = { navController.navigateToTab(destination.route) },
                colors =
                    NavigationBarItemDefaults.colors(
                        selectedIconColor = PokedexColors.Frame,
                        selectedTextColor = PokedexColors.Screen,
                        indicatorColor = PokedexColors.Screen,
                        unselectedIconColor = PokedexColors.CardStrong,
                        unselectedTextColor = PokedexColors.CardStrong,
                    ),
            )
        }
    }
}
